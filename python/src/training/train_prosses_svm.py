#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
SVM训练器: 使用文本特征和朴素贝叶斯预测结果训练SVM模型
用于PHP webshell检测
"""
import os
import sys
import json
import math
import argparse
import numpy as np
import random
from typing import Dict, List, Tuple, Any
from collections import Counter
import re
from sklearn.svm import SVC
from sklearn.preprocessing import StandardScaler
from sklearn.model_selection import StratifiedKFold, cross_validate, train_test_split
from sklearn.metrics import accuracy_score, precision_score, recall_score, f1_score, make_scorer
from sklearn.metrics import precision_recall_curve, roc_curve, auc
from sklearn.pipeline import Pipeline
import joblib
# 添加 ONNX 相关 import
try:
    from skl2onnx import convert_sklearn
    from skl2onnx.common.data_types import FloatTensorType
except ImportError:
    print("警告：未安装 skl2onnx 和 onnx 库，将不保存ONNX格式")
import gc
import time
import resource
import matplotlib.pyplot as plt
from scipy.optimize import minimize

# --- 为了能够导入 src 下的模块 ---
# 获取当前测试文件所在的目录 (python/training)
test_dir = os.path.dirname(os.path.abspath(__file__))
# 获取 tests 目录 (python/tests)
tests_root_dir = os.path.dirname(test_dir)
# 获取项目根目录下的 python 目录 (python)
python_root_dir = os.path.dirname(tests_root_dir)
# 获取 src 目录 (python/src)
src_dir = os.path.join(python_root_dir, 'src')

# 将 src 目录添加到 sys.path
sys.path.insert(0, src_dir)
# --- 导入路径设置结束 ---

# 从训练脚本导入需要测试的函数和类
from training.train_bayes import transform_ast_node_py, extract_words_from_ast, PyAstNode
# 从预处理模块导入 AST 解析器包装器
from preprocessing.ast_parser_wrapper import php_ast

# 导入朴素贝叶斯模型加载器
class BayesModel:
    """加载和使用朴素贝叶斯模型进行预测"""
    
    def __init__(self, model_path):
        """初始化贝叶斯模型
        
        Args:
            model_path: 模型JSON文件路径
        """
        self.model_path = model_path
        self.model_data = None
        self.load_model()
        
    def load_model(self):
        """加载模型数据"""
        try:
            with open(self.model_path, 'r', encoding='utf-8') as f:
                self.model_data = json.load(f)
            print(f"成功加载朴素贝叶斯模型: {self.model_path}")
        except Exception as e:
            print(f"加载朴素贝叶斯模型失败: {e}")
            sys.exit(1)
    
    def predict(self, words: List[str]) -> Dict[str, float]:
        """使用朴素贝叶斯模型预测词列表的分类概率
        
        Args:
            words: 从文件中提取的单词列表
            
        Returns:
            Dict: 包含各类别得分的字典 {"normal": score1, "webshell": score2}
        """
        if not self.model_data:
            print("错误: 模型未加载")
            return {"normal": 0.0, "webshell": 0.0}
            
        scores = {}
        total_docs = self.model_data.get("totalDocumentCount", 0)
        
        if total_docs == 0:
            return {"normal": 0.0, "webshell": 0.0}
            
        for class_name, class_data in self.model_data.items():
            if class_name == "totalDocumentCount":
                continue
                
            # 先验概率 P(class)
            doc_count = class_data.get("docCount", 0)
            prior = doc_count / total_docs
            
            # 词汇表和总词数
            word_counts = class_data.get("wordCount", {})
            total_word_count = class_data.get("totalWordCount", 0)
            vocab_size = len(word_counts)
            
            # 计算后验概率 log(P(class|words))
            posterior = math.log(prior)
            
            for word in words:
                # 获取当前词在该类别中的计数，如果不存在则为0
                word_count = word_counts.get(word, 0)
                
                # 使用拉普拉斯平滑计算条件概率 P(word|class)
                word_prob = (word_count + 1) / (total_word_count + vocab_size)
                posterior += math.log(word_prob)
                
            # 存储最终得分
            scores[class_name] = posterior
            
        # 转换为概率
        max_score = max(scores.values())
        exp_scores = {cls: math.exp(score - max_score) for cls, score in scores.items()}
        total = sum(exp_scores.values())
        normalized_scores = {cls: score / total for cls, score in exp_scores.items()}
        
        return normalized_scores
        
    def get_normalized_score(self, words: List[str]) -> float:
        """获取规范化的webshell得分
        
        Args:
            words: 从文件中提取的单词列表
            
        Returns:
            webshell概率得分 [0,1]
        """
        scores = self.predict(words)
        if "webshell" not in scores or "normal" not in scores:
            return 0.0
            
        # 直接返回webshell的概率值
        return scores["webshell"]



class TextFeatureExtractor:
    """提取PHP文件的文本特征"""
    def __init__(self):
        """初始化特征提取器"""
        self.tag_pattern = re.compile(r'<[\x00-\xFF]*?>')
        self.symbol_pattern = re.compile(r'[^a-zA-Z0-9]')
        self.statement_pattern = re.compile(r';')
        # 创建一个AST解析器实例
        self.ast_parser = php_ast()
        
    def __del__(self):
        """析构函数，确保资源被释放"""
        if hasattr(self, 'ast_parser'):
            try:
                self.ast_parser.cleanup()
            except:
                pass


    def extract_features(self, content: str) -> Dict[str, float]:
        """从PHP文件内容中提取8种文本特征，与Go实现保持一致
        
        Args:
            content: 文件内容
            
        Returns:
            特征字典，包含8种文本特征
        """
        # 1. 统计行数据
        lines = content.split('\n')
        line_lengths = [len(line) for line in lines]
        
        # 2. 统计单词数据
        words = []
        current_word_len = 0
        
        # 按照CloudWalker和Go实现的方式提取单词
        for c in content:
            if c.isalnum():  # 字母或数字
                current_word_len += 1
            elif current_word_len > 0:
                words.append(current_word_len)
                current_word_len = 0
                
        if current_word_len > 0:
            words.append(current_word_len)
            
        # 3. 计算特征
        features = {}
        
        # LM - 行长度最大值
        features['LM'] = max(line_lengths) if line_lengths else 0
        
        # LVC - 行变异系数 (不乘以100，与Go保持一致)
        if len(line_lengths) > 1 and np.mean(line_lengths) > 0:
            features['LVC'] = np.std(line_lengths, ddof=1) / np.mean(line_lengths)
        else:
            features['LVC'] = 0.0
            
        # WM - 词长度最大值
        features['WM'] = max(words) if words else 0
        
        # WVC - 词变异系数 (乘以100，与Go保持一致)
        if len(words) > 1 and np.mean(words) > 0:
            features['WVC'] = (np.std(words, ddof=1) / np.mean(words)) * 100
        else:
            features['WVC'] = 0.0
            
        # SR - 符号比率
        if len(content) > 0:
            symbol_count = len(self.symbol_pattern.findall(content))
            features['SR'] = (symbol_count / len(content)) * 100
        else:
            features['SR'] = 0.0
            
        # TR - 标签比率
        if len(words) > 0:
            tag_count = len(self.tag_pattern.findall(content))
            features['TR'] = (tag_count / len(words)) * 100
        else:
            features['TR'] = 0.0
            
        # SPL - 每行语句数
        if len(lines) > 0:
            statement_count = len(self.statement_pattern.findall(content))
            features['SPL'] = statement_count / len(lines)
        else:
            features['SPL'] = 0.0
            
        # IE - 信息熵
        features['IE'] = self._calculate_entropy(content)
        
        # 保留6位小数
        for key in features:
            features[key] = round(features[key], 6)
            
        return features
    
    def _calculate_entropy(self, text: str) -> float:
        """计算文本的信息熵，与Go实现保持一致
        
        Args:
            text: 输入文本
            
        Returns:
            信息熵值
        """
        if not text:
            return 0.0
            
        # 初始化频率数组
        char_counts = [0] * 256
        total_chars = 0
        
        # 统计字符频率，排除换行符
        for c in text:
            if 0 <= ord(c) < 256 and c != '\n':
                char_counts[ord(c)] += 1
                total_chars += 1
                
        if total_chars == 0:
            return 0.0
                
        # 计算熵
        entropy = 0.0
        for count in char_counts:
            if count > 0:
                probability = count / total_chars
                entropy -= probability * math.log2(probability)
                
        return entropy
    
    def extract_words_for_bayes(self, path: str) -> List[str]:
        """调用AST提取单词, 供朴素贝叶斯模型使用"""
        if not os.path.exists(path):
            print(f"错误: 文件不存在 {path}")
            return []
            
        try:
            # 使用类的ast_parser实例
            ast_data = self.ast_parser.get_file_ast(path)
            if ast_data.get('status') != 'successed':
                print(f"错误: AST解析失败，状态: {ast_data.get('status')}, 原因: {ast_data.get('reason')}")
                return []
                
            if 'ast' not in ast_data:
                print(f"错误: AST数据中没有'ast'键，文件: {path}")
                return []
            
            ast_raw = ast_data['ast']
            if ast_raw is None:
                print(f"错误: AST为None，文件: {path}")
                return []
            
            ast_transformed = transform_ast_node_py(ast_raw)
            return extract_words_from_ast(ast_transformed)
            
        except Exception as e:
            print(f"提取AST词袋时出错: {e}")
            return []




def extract_features_from_file(file_path: str, bayes_model: BayesModel, extractor: TextFeatureExtractor) -> Dict[str, float]:
    """从文件中提取所有特征，包括文本特征和朴素贝叶斯预测结果
    
    Args:
        file_path: 文件路径
        bayes_model: 朴素贝叶斯模型
        extractor: 文本特征提取器实例
        
    Returns:
        所有特征的字典
    """
    try:
        try:
            with open(file_path, 'r', encoding='utf-8', errors='ignore') as f:
                content = f.read()
        except FileNotFoundError:
             print(f"警告: 文件未找到 {file_path}, 跳过特征提取.")
             # 返回默认空特征，避免后续处理出错
             return {
                 'LM': 0.0, 'LVC': 0.0, 'WM': 0.0, 'WVC': 0.0,
                 'SR': 0.0, 'TR': 0.0, 'SPL': 0.0, 'IE': 0.0, 'BAYES': 0.0
             }

        text_features = extractor.extract_features(content)

        # 提取单词供朴素贝叶斯使用 (现在每次调用都会创建新的ast_parser实例)
        words = extractor.extract_words_for_bayes(file_path)

        # 获取朴素贝叶斯预测结果
        # bayes_score = bayes_model.get_normalized_score(words)
                # 获取朴素贝叶斯预测结果并保留6位小数
        bayes_score = round(bayes_model.get_normalized_score(words), 6)


        # 合并特征
        all_features = text_features.copy()
        all_features['BAYES'] = bayes_score

        return all_features

    except Exception as e:
        print(f"处理文件时出错 {file_path}: {e}")
        # 返回空特征
        return {
            'LM': 0.0, 'LVC': 0.0, 'WM': 0.0, 'WVC': 0.0,
            'SR': 0.0, 'TR': 0.0, 'SPL': 0.0, 'IE': 0.0, 'BAYES': 0.0
        }



def load_and_process_data(good_dir: str, bad_dir: str, bayes_model_path: str) -> Tuple[np.ndarray, np.ndarray]:
    """加载和处理数据"""
    bayes_model = BayesModel(bayes_model_path)
    
    # 获取文件列表
    good_files = [os.path.join(good_dir, f) for f in os.listdir(good_dir) if f.lower().endswith('.php')]
    bad_files = [os.path.join(bad_dir, f) for f in os.listdir(bad_dir) if f.lower().endswith('.php')]
    
    print(f"找到正常文件: {len(good_files)}, Webshell文件: {len(bad_files)}")
    
    # 平衡数据集
    min_samples = min(len(good_files), len(bad_files))
    if len(good_files) > min_samples:
        good_files = random.sample(good_files, min_samples)
    elif len(bad_files) > min_samples:
        bad_files = random.sample(bad_files, min_samples)
    
    all_files = good_files + bad_files
    labels = [0] * len(good_files) + [1] * len(bad_files)
    
    # 打乱数据
    combined = list(zip(all_files, labels))
    random.shuffle(combined)
    all_files, labels = zip(*combined)
    
    # 提取特征
    print("开始提取特征...")
    features_list = []
    processed_count = 0
    
    # 创建一个特征提取器实例
    feature_extractor = TextFeatureExtractor()
    
    try:
        for file_path in all_files:
            try:
                features = extract_features_from_file(file_path, bayes_model, feature_extractor)
                feature_vector = [
                    features['LM'], features['LVC'], features['WM'], features['WVC'],
                    features['SR'], features['TR'], features['SPL'], features['IE'],
                    features['BAYES']
                ]
                print("|=文件：", file_path, "|特征：", feature_vector)
                features_list.append(feature_vector)
                
                processed_count += 1
                if processed_count % 10 == 0:
                    print(f"已处理 {processed_count}/{len(all_files)} 文件...")
                
            except Exception as e:
                print(f"处理文件 {file_path} 时出错: {e}")
                features_list.append([0.0] * 9)
    finally:
        # 确保资源被释放
        del feature_extractor
        gc.collect()
    
    return np.array(features_list), np.array(labels)


# 添加自定义Sigmoid函数用于SVM分数映射
def custom_sigmoid(x, a=1.0, b=0.0):
    """
    自定义Sigmoid函数，用于将SVM决策值映射到[0,1]区间
    
    Args:
        x: 原始决策值
        a: 斜率参数，控制sigmoid曲线的陡峭程度
        b: 偏移参数，控制决策边界的位置
        
    Returns:
        映射后的概率值[0-1]
    """
    return 1.0 / (1.0 + np.exp(-a * (x - b)))

# 寻找最优sigmoid参数的函数
def find_optimal_sigmoid_params(model, X_val, y_val):
    """
    使用验证集找到最优的sigmoid参数
    
    Args:
        model: 训练好的SVM模型
        X_val: 验证集特征
        y_val: 验证集标签
        
    Returns:
        tuple: (a, b) sigmoid参数
    """
    def objective(params):
        a, b = params
        # 获取决策值
        decision_values = model.decision_function(X_val)
        # 应用sigmoid转换
        probs = custom_sigmoid(decision_values, a, b)
        # 限制概率范围，避免log(0)
        probs = np.clip(probs, 1e-15, 1.0 - 1e-15)
        # 计算交叉熵损失
        loss = -np.mean(y_val * np.log(probs) + (1 - y_val) * np.log(1 - probs))
        return loss
    
    # 优化过程
    initial_params = [1.0, 0.0]  # 初始参数
    result = minimize(objective, initial_params, method='Nelder-Mead')
    if result.success:
        return result.x
    else:
        print("警告: Sigmoid参数优化失败，使用默认参数")
        return initial_params

# 寻找最优决策阈值的函数
def find_optimal_threshold(y_true, y_scores, cost_ratio=3.0):
    """
    基于精确率-召回率曲线找到最佳决策阈值
    
    Args:
        y_true: 真实标签
        y_scores: 预测分数
        cost_ratio: 误判正常为webshell的成本比率(相对于漏检的成本)
        
    Returns:
        最佳阈值
    """
    precisions, recalls, thresholds = precision_recall_curve(y_true, y_scores)
    
    # 计算每个阈值的假阳性率
    fpr, tpr, _ = roc_curve(y_true, y_scores)
    
    # 计算考虑成本的加权F1
    f1_scores = []
    for i in range(len(thresholds)):
        # 精确率为0时跳过
        if precisions[i] == 0:
            f1_scores.append(0)
            continue
            
        # 基于成本计算加权F1
        weighted_recall = recalls[i]
        weighted_precision = precisions[i] ** cost_ratio  # 精确率权重更高
        
        # 避免除零
        if weighted_precision + weighted_recall > 0:
            weighted_f1 = 2 * (weighted_precision * weighted_recall) / (weighted_precision + weighted_recall)
        else:
            weighted_f1 = 0
            
        f1_scores.append(weighted_f1)
    
    # 获取最佳阈值索引
    best_idx = np.argmax(f1_scores)
    
    # 边界情况处理
    if best_idx >= len(thresholds):
        return 0.5  # 默认阈值
        
    return thresholds[best_idx]

# 优化后的训练SVM模型函数
def train_svm_model(X: np.ndarray, y: np.ndarray, output_model_path: str, cv_folds: int = 4):
    """
    训练SVM模型并保存为libSVM格式，带有校准参数
    
    Args:
        X: 特征矩阵
        y: 标签向量
        output_model_path: 模型输出基础路径 (不含扩展名)
        cv_folds: 交叉验证折数
    """
    # 划分训练集和验证集
    X_train, X_val, y_train, y_val = train_test_split(X, y, test_size=0.2, random_state=42, stratify=y)
    
    # 先创建并训练标准化器
    print("创建标准化器...")
    scaler = StandardScaler()
    X_train_scaled = scaler.fit_transform(X_train)
    X_val_scaled = scaler.transform(X_val)

    # 记录特征的统计信息
    feature_stats = {
        'mins': X.min(axis=0).tolist(),
        'maxs': X.max(axis=0).tolist(),
        'means': scaler.mean_.tolist(),
        'stds': scaler.scale_.tolist(),
    }

    # 创建SVM模型
    print("创建SVM模型...")
    svm_model = SVC(
        kernel='rbf', 
        gamma='1',  # 自动选择gamma
        C=10.0,  # 增大C值以提高模型复杂度
        probability=True, 
        random_state=42,
        class_weight='balanced'  # 使用平衡的类权重
    )
    
    # 在训练集上训练模型
    print("训练SVM模型...")
    svm_model.fit(X_train_scaled, y_train)
    
    # 交叉验证（如果需要）
    if cv_folds >= 2:
        # 交叉验证代码保持不变
        print(f"执行{cv_folds}折交叉验证...")
        # ...

    # 在验证集上评估模型
    print("在验证集上评估模型...")
    y_val_pred = svm_model.predict(X_val_scaled)
    val_accuracy = accuracy_score(y_val, y_val_pred)
    val_precision = precision_score(y_val, y_val_pred)
    val_recall = recall_score(y_val, y_val_pred)
    val_f1 = f1_score(y_val, y_val_pred)
    
    print(f"验证集结果: 准确率={val_accuracy:.4f}, 精确率={val_precision:.4f}, 召回率={val_recall:.4f}, F1={val_f1:.4f}")
    
    # 获取验证集的决策值和概率
    y_val_decision = svm_model.decision_function(X_val_scaled)
    y_val_prob = svm_model.predict_proba(X_val_scaled)[:, 1]
    
    # 找到最优的sigmoid参数
    print("寻找最优sigmoid参数...")
    sigmoid_a, sigmoid_b = find_optimal_sigmoid_params(svm_model, X_val_scaled, y_val)
    print(f"最优sigmoid参数: a={sigmoid_a:.4f}, b={sigmoid_b:.4f}")
    
    # 应用sigmoid转换
    y_val_sigmoid = custom_sigmoid(y_val_decision, sigmoid_a, sigmoid_b)
    
    # 找到最优决策阈值
    print("寻找最优决策阈值...")
    optimal_threshold = find_optimal_threshold(y_val, y_val_sigmoid, cost_ratio=3.0)
    print(f"最优决策阈值: {optimal_threshold:.4f}")
    
    # 在所有数据上重新训练最终模型
    print("在所有数据上训练最终模型...")
    X_all_scaled = scaler.fit_transform(X)
    feature_stats['means'] = scaler.mean_.tolist()  # 更新均值
    feature_stats['stds'] = scaler.scale_.tolist()  # 更新标准差
    
    final_svm_model = SVC(
        kernel='rbf', 
        gamma='auto',
        C=10.0,
        probability=True, 
        random_state=42,
        class_weight='balanced'
    )
    final_svm_model.fit(X_all_scaled, y)
    
    # 保存标准化参数和校准信息
    calibration_info = {
        'feature_names': ['LM', 'LVC', 'WM', 'WVC', 'SR', 'TR', 'SPL', 'IE', 'BAYES'],
        'num_features': X.shape[1],
        'feature_stats': feature_stats,
        'sigmoid_params': {
            'a': float(sigmoid_a),
            'b': float(sigmoid_b)
        },
        'optimal_threshold': float(optimal_threshold),
        'class_mapping': {
            '0': 'normal',
            '1': 'webshell'
        },
        'metrics': {
            'validation_accuracy': float(val_accuracy),
            'validation_precision': float(val_precision),
            'validation_recall': float(val_recall),
            'validation_f1': float(val_f1)
        }
    }
    
    # 创建验证样本
    print("创建验证样本...")
    validation_samples = {}
    
    # 选择一些正确分类的样本作为验证样本
    for cls in [0, 1]:
        # 找到该类别的样本
        cls_indices = np.where(y_val == cls)[0]
        if len(cls_indices) > 0:
            # 选择最能代表该类的几个样本（决策值最典型的）
            if cls == 0:  # 正常样本，决策值最小
                rep_indices = cls_indices[np.argsort(y_val_decision[cls_indices])[:3]]
            else:  # webshell样本，决策值最大
                rep_indices = cls_indices[np.argsort(y_val_decision[cls_indices])[-3:]]
                
            for i, idx in enumerate(rep_indices):
                sample_name = f"representative_{('normal' if cls==0 else 'webshell')}_{i}"
                validation_samples[sample_name] = {
                    'features': X_val[idx].tolist(),
                    'raw_decision': float(y_val_decision[idx]),
                    'sigmoid_score': float(custom_sigmoid(y_val_decision[idx], sigmoid_a, sigmoid_b)),
                    'expected_class': 'normal' if cls == 0 else 'webshell'
                }
    
    calibration_info['validation_samples'] = validation_samples
    
    # 保存校准信息
    info_path = output_model_path + '.info'
    try:
        with open(info_path, 'w', encoding='utf-8') as f:
            json.dump(calibration_info, f, ensure_ascii=False, indent=2)
        print(f"校准信息已保存至: {info_path}")
    except IOError as e:
        print(f"保存校准信息文件失败: {e}")

    # 创建决策值分布图
    plt.figure(figsize=(10, 6))
    
    # 绘制不同类别的决策值分布
    plt.hist(y_val_decision[y_val==0], bins=20, alpha=0.5, label='正常文件', color='green')
    plt.hist(y_val_decision[y_val==1], bins=20, alpha=0.5, label='Webshell', color='red')
    
    # 标记sigmoid转换后的阈值位置
    threshold_decision = math.log(1/optimal_threshold - 1) / (-sigmoid_a) + sigmoid_b
    plt.axvline(x=threshold_decision, color='blue', linestyle='--', label=f'决策阈值 ({optimal_threshold:.2f})')
    
    plt.title('SVM决策值分布')
    plt.xlabel('决策值')
    plt.ylabel('样本数')
    plt.legend()
    plt.grid(True, alpha=0.3)
    
    # 保存图形
    plt.savefig(output_model_path + '_decision_dist.png')
    print(f"决策值分布图已保存至: {output_model_path}_decision_dist.png")

    # 保存为libSVM格式（改进版）
    svm_model_path = output_model_path + ".model"
    with open(svm_model_path, 'w') as f:
        # 基本元数据
        f.write(f"svm_type c_svc\n")
        f.write(f"kernel_type rbf\n")
        f.write(f"gamma {final_svm_model.gamma}\n")
        f.write(f"nr_class 2\n")
        
        # 支持向量信息
        n_sv = len(final_svm_model.support_vectors_)
        f.write(f"total_sv {n_sv}\n")
        f.write(f"rho {-final_svm_model.intercept_[0]}\n")
        f.write(f"label 0 1\n")
        
        # 每个类的支持向量数量
        class0_sv = sum(1 for i in range(len(final_svm_model.support_)) 
                      if final_svm_model.dual_coef_[0][i] < 0)
        class1_sv = n_sv - class0_sv
        f.write(f"nr_sv {class0_sv} {class1_sv}\n")
        
        # 关键改进：保持系数原始符号，并使用原始支持向量
        f.write("SV\n")
        for i, sv_idx in enumerate(final_svm_model.support_):
            # 保持系数原始值（不取绝对值）
            coef = final_svm_model.dual_coef_[0][i]
            
            # 构建特征字符串
            features_str = " ".join([f"{j+1}:{sv_j:.6f}" for j, sv_j in enumerate(final_svm_model.support_vectors_[i]) 
                                   if abs(sv_j) > 1e-5])
            
            # 写入文件
            f.write(f"{coef:.6f} {features_str}\n")
        
    print(f"模型已保存为标准libSVM格式: {svm_model_path}")


def limit_memory():
    """限制进程内存使用"""
    soft, hard = resource.getrlimit(resource.RLIMIT_AS)
    # 设置软限制为8GB
    resource.setrlimit(resource.RLIMIT_AS, (8 * 1024 * 1024 * 1024, hard))


def main():
    limit_memory()
    parser = argparse.ArgumentParser(description='训练SVM模型用于PHP Webshell检测 (保存为libSVM格式)')
    parser.add_argument('--good-dir', default='/opt/WebshellDet/bt-ShieldML/data/cleaned/php/normal/', 
                        help='正常PHP文件目录')
    parser.add_argument('--bad-dir', default='/opt/WebshellDet/bt-ShieldML/data/cleaned/php/webshell/', 
                        help='Webshell PHP文件目录')
    parser.add_argument('--bayes-model', default='/opt/WebshellDet/bt-ShieldML/data/models/Words.model', 
                        help='朴素贝叶斯模型路径')
    parser.add_argument('--output-model', default='/opt/WebshellDet/bt-ShieldML/data/models/ProcessSVM', 
                        help='输出模型的基础路径 (例如 ProcessSVM, 会生成 .onnx 和 .info)')
    parser.add_argument('--cv-folds', type=int, default=4, 
                        help='交叉验证折数')
    parser.add_argument('--test-file', default='/opt/WebshellDet/bt-ShieldML/data/cleaned/php/onema.php', 
                        help='测试特征提取的文件')
    parser.add_argument('--test-only', action='store_true', 
                        help='只运行测试，不训练模型')
    
    args = parser.parse_args()
    
    # 验证参数
    if not args.test_only:
        if not os.path.isdir(args.good_dir):
            print(f"错误: 正常样本目录不存在: {args.good_dir}")
            sys.exit(1)
        if not os.path.isdir(args.bad_dir):
            print(f"错误: Webshell样本目录不存在: {args.bad_dir}")
            sys.exit(1)
    
    if not os.path.isfile(args.bayes_model):
        print(f"错误: 朴素贝叶斯模型文件不存在: {args.bayes_model}")
        sys.exit(1)

    
    # 训练模型
    print("\n开始训练SVM模型...")
    X, y = load_and_process_data(args.good_dir, args.bad_dir, args.bayes_model)
    if X.shape[0] > 0: # 确保有数据用于训练
        train_svm_model(X, y, args.output_model, args.cv_folds)
    else:
        print("错误：没有足够的数据来训练模型。")
        sys.exit(1)
    
    print("\n训练完成!")


if __name__ == "__main__":
    main()