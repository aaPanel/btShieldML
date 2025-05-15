# path: python/src/training/train_bayes.py
import os
import json
import argparse
import sys
import random
import numpy as np
from typing import List, Dict, Any, Tuple, Optional

from collections import Counter
# 确保可以导入同级目录下的模块
current_dir = os.path.dirname(os.path.abspath(__file__))
src_dir = os.path.dirname(current_dir)
preprocessing_dir = os.path.join(src_dir, 'preprocessing')
sys.path.insert(0, src_dir)

from sklearn.feature_extraction.text import TfidfVectorizer
from sklearn.naive_bayes import MultinomialNB, ComplementNB
from sklearn.model_selection import StratifiedKFold, cross_validate, train_test_split
from sklearn.metrics import make_scorer, precision_score, recall_score, f1_score, accuracy_score, confusion_matrix
from preprocessing.ast_parser_wrapper import php_ast

# --- AST Node Class 和 AST Transformation Logic 保持不变 ---
class PyAstNode:
    """Represents a node in the PHP AST, mimicking Go's astNode."""
    def __init__(self, kind: int, flags: int, lineno: int, children: Any):
        self.kind = kind
        self.flags = flags
        self.lineno = lineno
        self.children = children

    def __repr__(self):
        child_repr = "..." if self.children is not None else "None"
        return f"PyAstNode(kind={self.kind}, flags={self.flags}, lineno={self.lineno}, children={child_repr})"

def get_int(v: Any, default: int = 0) -> int:
    """Helper to safely convert potential float/None/str to int."""
    if isinstance(v, (int, float)):
        return int(v)
    return default

def transform_ast_node_py(node_data: Any) -> Any:
    """
    Transforms raw parsed AST data (dicts/lists/primitives)
    into PyAstNode objects where applicable, mimicking Go's transformAstNode.
    """
    if node_data is None:
        return None

    # Keep basic types as is
    if isinstance(node_data, (str, bool, int, float)):
        return node_data

    # Recursively transform list elements
    if isinstance(node_data, list):
        return [transform_ast_node_py(item) for item in node_data]

    # Process dictionaries
    if isinstance(node_data, dict):
        # Check if it looks like an AST node dictionary (must have 'kind')
        kind_val = node_data.get('kind')
        # Try converting kind to int. If successful, assume it's an AST node.
        try:
            if kind_val is not None:
                 kind = get_int(kind_val, -999)
                 if kind != -999:
                    flags = get_int(node_data.get('flags'))
                    lineno = get_int(node_data.get('lineno'))
                    children_raw = node_data.get('children')
                    children_transformed = transform_ast_node_py(children_raw)
                    return PyAstNode(kind, flags, lineno, children_transformed)
        except (ValueError, TypeError):
            pass

        # If it's not an AST node dict, transform its values recursively
        transformed_dict = {}
        for k, v in node_data.items():
            transformed_dict[k] = transform_ast_node_py(v)
        return transformed_dict

    # Fallback for any other unhandled types
    print(f"警告: AST变换中的未处理类型: {type(node_data)}")
    return node_data

# --- 优化AST词袋提取函数，增加关键函数权重 ---
def extract_words_from_ast(node: Any, dangerous_funcs: Dict[str, float] = None) -> List[str]:
    """
    递归提取AST节点中的名称字段，并对危险函数进行权重处理
    Args:
        node: AST节点
        dangerous_funcs: 危险函数字典，包含函数名和权重
    Returns:
        提取的词列表，危险函数会根据权重被重复添加
    """
    words = []
    if dangerous_funcs is None:
        dangerous_funcs = {
            # 执行代码的函数
            "eval": 5.0, "assert": 5.0, "create_function": 5.0, "exec": 5.0,
            "passthru": 4.0, "system": 4.0, "shell_exec": 4.0, "proc_open": 4.0, 
            "popen": 4.0, "pcntl_exec": 4.0, "call_user_func": 3.0,
            
            # 编码/解码函数
            "base64_decode": 3.0, "str_rot13": 3.0, "gzinflate": 3.0, 
            "gzuncompress": 3.0, "gzdecode": 3.0, "str_replace": 2.0,
            
            # 文件操作函数
            "file_get_contents": 2.0, "file_put_contents": 2.0, "fopen": 2.0,
            "fwrite": 2.0, "file": 2.0, "fputs": 2.0, "unlink": 2.0,
            
            # 网络功能
            "fsockopen": 3.0, "curl_exec": 3.0, "curl_init": 2.0
        }
    
    if isinstance(node, PyAstNode):
        # 如果是AST节点，检查children
        if isinstance(node.children, dict):
            name_val = node.children.get('name')
            if isinstance(name_val, str):
                words.append(name_val)
                # 如果是危险函数，根据权重添加多次
                if name_val in dangerous_funcs:
                    weight = int(dangerous_funcs[name_val])
                    # 添加前缀标记，确保Go端也认识这个特殊标记
                    words.append(f"DANGER_FUNC_{name_val}")
                    # 根据权重重复添加
                    for _ in range(weight - 1):
                        words.append(name_val)
        
        # 继续递归处理children
        words.extend(extract_words_from_ast(node.children, dangerous_funcs))

    elif isinstance(node, dict):
        # 字典情况
        name_val = node.get('name')
        if isinstance(name_val, str):
            words.append(name_val)
            if name_val in dangerous_funcs:
                weight = int(dangerous_funcs[name_val])
                words.append(f"DANGER_FUNC_{name_val}")
                for _ in range(weight - 1):
                    words.append(name_val)
                
        # 递归字典的所有值
        for key, value in node.items():
            words.extend(extract_words_from_ast(value, dangerous_funcs))

    elif isinstance(node, list):
        # 列表情况
        for item in node:
            words.extend(extract_words_from_ast(item, dangerous_funcs))

    return words

# --- 模型保存逻辑保持不变 ---
def save_model_for_go_bayesian(classifier: MultinomialNB, vectorizer: TfidfVectorizer, classes: List[str], output_path: str):
    """
    保存贝叶斯模型以供Go使用
    """
    if not isinstance(classifier, (MultinomialNB, ComplementNB)):
        print(f"警告: 保存逻辑主要为MultinomialNB/ComplementNB验证。当前类型: {type(classifier)}")

    if len(classes) != 2:
        print("错误: 需要恰好两个输出类名称 (例如 ['normal', 'webshell'])。")
        return

    label_normal = 0
    label_webshell = 1
    internal_classes = list(classifier.classes_)
    try:
        idx_normal = internal_classes.index(label_normal)
        idx_webshell = internal_classes.index(label_webshell)
    except ValueError:
        print(f"错误: 分类器未使用标签 ({label_normal}, {label_webshell}) 训练。学习到的类别: {internal_classes}")
        return

    if not hasattr(classifier, 'feature_count_'):
         print(f"错误: {type(classifier)} 类型的分类器没有 'feature_count_' 属性。无法按预期格式保存。")
         return
    if not hasattr(classifier, 'class_count_'):
        print(f"错误: {type(classifier)} 类型的分类器没有 'class_count_' 属性。无法按预期格式保存。")
        return

    total_docs = classifier.class_count_.sum()
    docs_normal = classifier.class_count_[idx_normal]
    docs_webshell = classifier.class_count_[idx_webshell]

    words_count_normal: Dict[str, int] = {}
    words_count_webshell: Dict[str, int] = {}
    vocab = vectorizer.get_feature_names_out()
    feature_counts = classifier.feature_count_

    total_words_normal = 0
    total_words_webshell = 0

    for i, word in enumerate(vocab):
        count_normal = int(feature_counts[idx_normal, i])
        count_webshell = int(feature_counts[idx_webshell, i])
        if count_normal > 0:
            words_count_normal[word] = count_normal
            total_words_normal += count_normal
        if count_webshell > 0:
            words_count_webshell[word] = count_webshell
            total_words_webshell += count_webshell

    output_class_normal, output_class_webshell = classes[0], classes[1]

    go_model_data = {
        output_class_normal: {
            "docCount": int(docs_normal),
            "wordCount": words_count_normal,
            "totalWordCount": int(total_words_normal)
        },
        output_class_webshell: {
            "docCount": int(docs_webshell),
            "wordCount": words_count_webshell,
            "totalWordCount": int(total_words_webshell)
        },
        "totalDocumentCount": int(total_docs),
    }

    try:
        with open(output_path, 'w', encoding='utf-8') as f:
            json.dump(go_model_data, f, ensure_ascii=False, indent=4)
        print(f"模型已成功保存为Go贝叶斯格式到: {output_path}")
    except IOError as e:
        print(f"保存模型到 {output_path} 时出错: {e}")

def main():
    parser = argparse.ArgumentParser(description="训练基于AST词的朴素贝叶斯分类器，并保存为Go贝叶斯格式。")
    parser.add_argument("--good-dir", required=True, help="包含正常PHP文件的目录 (标签 0)。")
    parser.add_argument("--bad-dir", required=True, help="包含webshell PHP文件的目录 (标签 1)。")
    parser.add_argument("--output-model", default="Words.model", help="保存训练模型的路径。")
    parser.add_argument("--php-version", default="7", choices=["5", "7", "8"], help="AST解析器使用的PHP版本。")
    parser.add_argument("--cv-folds", type=int, default=4, help="交叉验证的折数。")
    # 新增参数
    parser.add_argument("--classifier", default="complement", choices=["multinomial", "complement"], 
                        help="使用的朴素贝叶斯分类器类型，complement对不平衡数据效果更好。")
    parser.add_argument("--sample-ratio", type=float, default=0.2, 
                        help="恶意样本与正常样本的比例 (例如0.2表示每5个正常样本对应1个恶意样本)")
    parser.add_argument("--threshold", type=float, default=0.7, 
                        help="恶意判断阈值，保存在模型中，供Go端使用")
    parser.add_argument("--test-size", type=float, default=0.2,
                        help="用于测试的数据比例")
    args = parser.parse_args()

    if not os.path.isdir(args.good_dir):
        print(f"错误: 正常样本目录未找到: {args.good_dir}")
        sys.exit(1)
    if not os.path.isdir(args.bad_dir):
        print(f"错误: Webshell样本目录未找到: {args.bad_dir}")
        sys.exit(1)

    # 初始化PHP AST解析器
    ast_parser = php_ast()
    try:
        pass
    except Exception as e:
        print(f"初始化PHP运行时桥接时出错: {e}")
        sys.exit(1)

    # --- 改进: 加载文件并创建更真实的样本比例 ---
    print(f"加载初始文件列表...")
    good_files = [os.path.join(args.good_dir, f) for f in os.listdir(args.good_dir) if f.lower().endswith(".php")]
    bad_files = [os.path.join(args.bad_dir, f) for f in os.listdir(args.bad_dir) if f.lower().endswith(".php")]

    # 计算初始数量
    n_good_initial = len(good_files)
    n_bad_initial = len(bad_files)
    print(f"初始数量: 正常文件 = {n_good_initial}, Webshell文件 = {n_bad_initial}")

    if n_good_initial == 0 or n_bad_initial == 0:
        print("错误: 一个或两个样本目录都不包含PHP文件。")
        sys.exit(1)

    # 根据指定的样本比例调整样本数量
    target_ratio = args.sample_ratio  # 恶意/正常比例
    
    # 确保保留所有恶意样本（少数类）
    if n_good_initial * target_ratio > n_bad_initial:
        # 如果恶意样本太少，调整正常样本数量
        n_good_sampled = int(n_bad_initial / target_ratio)
        print(f"基于目标比例 {target_ratio}，对正常文件进行欠采样，从 {n_good_initial} 减少到 {n_good_sampled}...")
        good_files_sampled = random.sample(good_files, n_good_sampled)
        bad_files_sampled = bad_files
    else:
        # 保留所有正常样本，恶意样本不太可能超过所需比例
        good_files_sampled = good_files
        bad_files_sampled = bad_files
        print(f"使用所有可用样本，实际比例约为 {n_bad_initial/n_good_initial:.4f}")

    # 分出训练集和测试集
    good_train, good_test = train_test_split(good_files_sampled, test_size=args.test_size, random_state=42)
    bad_train, bad_test = train_test_split(bad_files_sampled, test_size=args.test_size, random_state=42)
    
    # 构建训练和测试文件列表
    train_files = good_train + bad_train
    train_labels = [0] * len(good_train) + [1] * len(bad_train)
    test_files = good_test + bad_test
    test_labels = [0] * len(good_test) + [1] * len(bad_test)
    
    # 打乱训练数据顺序
    train_combined = list(zip(train_files, train_labels))
    random.shuffle(train_combined)
    train_files, train_labels = zip(*train_combined)
    train_files = list(train_files)
    train_labels = list(train_labels)
    
    print(f"训练集: {len(good_train)} 正常文件, {len(bad_train)} webshell文件")
    print(f"测试集: {len(good_test)} 正常文件, {len(bad_test)} webshell文件")

    # 处理训练文件
    print("提取训练集AST特征...")
    train_corpus = []
    processed_count = 0
    
    for filepath in train_files:
        try:
            ast_data = ast_parser.get_file_ast(filepath)
            
            if ast_data and ast_data.get('status') == 'successed' and 'ast' in ast_data:
                ast_raw = ast_data['ast']
                ast_transformed = transform_ast_node_py(ast_raw)
                
                if ast_transformed:
                    # 使用优化后的词袋提取函数
                    words = extract_words_from_ast(ast_transformed)
                    train_corpus.append(" ".join(words))
                else:
                    print(f"警告: AST变换失败或结果为None: {filepath}")
                    train_corpus.append("")
            else:
                status = ast_data.get('status', 'N/A') if ast_data else 'N/A'
                reason = ast_data.get('reason', 'N/A') if ast_data else 'N/A'
                print(f"警告: 无法获取有效AST: {filepath}. 状态: {status}, 原因: {reason}")
                train_corpus.append("")
        except Exception as e:
            print(f"处理文件 {filepath} 时出错: {e}")
            train_corpus.append("")
            
        processed_count += 1
        if processed_count % 100 == 0:
            print(f"  已处理 {processed_count}/{len(train_files)} 训练文件...")
    
    # 处理测试文件
    print("提取测试集AST特征...")
    test_corpus = []
    processed_count = 0
    
    for filepath in test_files:
        try:
            ast_data = ast_parser.get_file_ast(filepath)
            
            if ast_data and ast_data.get('status') == 'successed' and 'ast' in ast_data:
                ast_raw = ast_data['ast']
                ast_transformed = transform_ast_node_py(ast_raw)
                
                if ast_transformed:
                    words = extract_words_from_ast(ast_transformed)
                    test_corpus.append(" ".join(words))
                else:
                    test_corpus.append("")
            else:
                test_corpus.append("")
        except Exception as e:
            print(f"处理文件 {filepath} 时出错: {e}")
            test_corpus.append("")
            
        processed_count += 1
        if processed_count % 100 == 0:
            print(f"  已处理 {processed_count}/{len(test_files)} 测试文件...")

    print("特征提取完成，进行向量化...")

    # 使用TF-IDF向量化
    vectorizer = TfidfVectorizer(lowercase=False, token_pattern=r"[^\s]+")
    X_train = vectorizer.fit_transform(train_corpus)
    y_train = train_labels
    
    # 转换测试集
    X_test = vectorizer.transform(test_corpus)
    y_test = test_labels

    print(f"词汇表大小: {len(vectorizer.vocabulary_)}")
    if X_train.shape[0] == 0 or X_train.shape[1] == 0:
        print("错误: 特征矩阵为空。无法训练模型。检查AST提取和向量化。")
        sys.exit(1)

    # 根据参数选择分类器类型
    print(f"训练{args.classifier}朴素贝叶斯模型...")
    if args.classifier == "complement":
        # ComplementNB对不平衡数据效果更好
        classifier = ComplementNB(alpha=1.0)  # 适当增加平滑参数
    else:
        classifier = MultinomialNB(alpha=1.0)
        
    try:
        classifier.fit(X_train, y_train)
    except Exception as e:
        print(f"模型拟合过程中出错: {e}")
        print("检查标签和语料库/X是否对齐，或X是否有效。")
        sys.exit(1)

    # 评估模型在测试集上的性能
    print("\n在测试集上评估模型性能...")
    y_pred = classifier.predict(X_test)
    y_pred_proba = classifier.predict_proba(X_test)[:, 1]  # 取第1类(webshell)的概率
    
    # 计算基本性能指标
    accuracy = accuracy_score(y_test, y_pred)
    precision = precision_score(y_test, y_pred, zero_division=0)
    recall = recall_score(y_test, y_pred, zero_division=0)
    f1 = f1_score(y_test, y_pred, zero_division=0)
    
    # 计算混淆矩阵
    tn, fp, fn, tp = confusion_matrix(y_test, y_pred).ravel()
    
    print(f"准确率: {accuracy:.4f}")
    print(f"精确率: {precision:.4f}")
    print(f"召回率: {recall:.4f}")
    print(f"F1分数: {f1:.4f}")
    print(f"混淆矩阵:")
    print(f"真负例(TN): {tn} | 假正例(FP): {fp}")
    print(f"假负例(FN): {fn} | 真正例(TP): {tp}")
    
    # 假阳性率(FPR)和假阴性率(FNR)
    fpr = fp / (fp + tn) if (fp + tn) > 0 else 0
    fnr = fn / (fn + tp) if (fn + tp) > 0 else 0
    print(f"假阳性率: {fpr:.4f}")
    print(f"假阴性率: {fnr:.4f}")
    
    # 阈值分析
    print("\n基于不同阈值的性能分析:")
    thresholds = [0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9]
    best_threshold = args.threshold  # 默认阈值
    best_f1 = 0
    
    for threshold in thresholds:
        y_pred_t = (y_pred_proba >= threshold).astype(int)
        prec = precision_score(y_test, y_pred_t, zero_division=0)
        rec = recall_score(y_test, y_pred_t, zero_division=0)
        f1_t = f1_score(y_test, y_pred_t, zero_division=0)
        
        # 计算该阈值下的混淆矩阵
        tn_t, fp_t, fn_t, tp_t = confusion_matrix(y_test, y_pred_t).ravel()
        fpr_t = fp_t / (fp_t + tn_t) if (fp_t + tn_t) > 0 else 0
        
        print(f"阈值 {threshold:.1f}: 精确率={prec:.4f}, 召回率={rec:.4f}, F1={f1_t:.4f}, FPR={fpr_t:.4f}")
        
        # 更新最佳阈值(优先考虑F1分数，同时关注假阳性率)
        if f1_t > best_f1 and fpr_t < 0.1:  # 限制假阳性率不超过10%
            best_f1 = f1_t
            best_threshold = threshold
    
    print(f"\n推荐阈值: {best_threshold:.2f} (F1={best_f1:.4f})")
    
    # 在全部数据上重新训练模型
    print("\n使用所有数据训练最终模型...")
    # 合并训练集和测试集
    all_corpus = train_corpus + test_corpus
    all_labels = train_labels + test_labels
    
    # 重新向量化和训练
    final_vectorizer = TfidfVectorizer(lowercase=False, token_pattern=r"[^\s]+")
    X_all = final_vectorizer.fit_transform(all_corpus)
    
    if args.classifier == "complement":
        final_classifier = ComplementNB(alpha=1.0)
    else:
        final_classifier = MultinomialNB(alpha=1.0)
    
    final_classifier.fit(X_all, all_labels)

    # 保存模型
    print("保存最终模型...")
    class_names = ["normal", "webshell"]
    output_dir = os.path.dirname(args.output_model)
    if output_dir and not os.path.exists(output_dir):
         print(f"创建输出目录: {output_dir}")
         os.makedirs(output_dir)
    
    # 修改保存函数以包含阈值信息
    save_model_for_go_bayesian(final_classifier, final_vectorizer, class_names, args.output_model)
    
    # 额外保存阈值到独立配置文件
    threshold_config = {
        "model": os.path.basename(args.output_model),
        "threshold": best_threshold,
        "metrics": {
            "accuracy": accuracy,
            "precision": precision,
            "recall": recall,
            "f1": f1,
            "fpr": fpr,
            "fnr": fnr
        }
    }
    
    threshold_path = os.path.join(os.path.dirname(args.output_model), "bayes_threshold.json")
    try:
        with open(threshold_path, 'w', encoding='utf-8') as f:
            json.dump(threshold_config, f, ensure_ascii=False, indent=4)
        print(f"阈值配置已保存到: {threshold_path}")
    except Exception as e:
        print(f"保存阈值配置时出错: {e}")
    
    print("训练完成。")

if __name__ == "__main__":
    main()
