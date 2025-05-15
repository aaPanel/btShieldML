#!/usr/bin/env python3
# path: python/src/checking/check_bayes.py
"""
使用训练好的贝叶斯模型检测PHP文件是否为WebShell
"""

import os
import sys
import json
import argparse
import time
from typing import List, Dict, Any, Tuple, Optional

# 确保可以导入同级目录下的模块
current_dir = os.path.dirname(os.path.abspath(__file__))
src_dir = os.path.dirname(current_dir)
sys.path.insert(0, src_dir)

# 导入AST处理相关函数
from training.train_bayes import transform_ast_node_py, extract_words_from_ast, PyAstNode
from preprocessing.ast_parser_wrapper import php_ast

class BayesModelChecker:
    """朴素贝叶斯模型检测器，用于检测PHP文件是否为WebShell"""
    
    def __init__(self, model_path: str, threshold_path: str = None, verbose: bool = False):
        """
        初始化检测器
        
        Args:
            model_path: 模型文件路径
            threshold_path: 阈值配置文件路径（可选）
            verbose: 是否输出详细信息
        """
        self.model_path = model_path
        self.threshold = 0.7  # 默认阈值
        self.model_data = None
        self.verbose = verbose
        
        # 初始化AST解析器
        self.ast_parser = php_ast()
        
        # 初始化危险函数列表
        self.dangerous_funcs = {
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
        
        # 加载模型
        self.load_model()
        
        # 加载阈值配置
        if threshold_path:
            self.load_threshold(threshold_path)
        else:
            # 自动查找同目录下的阈值配置
            auto_threshold_path = os.path.join(os.path.dirname(model_path), "bayes_threshold.json")
            if os.path.exists(auto_threshold_path):
                self.load_threshold(auto_threshold_path)
    
    def load_threshold(self, threshold_path: str):
        """加载阈值配置文件"""
        try:
            with open(threshold_path, 'r', encoding='utf-8') as f:
                config = json.load(f)
                if "threshold" in config and isinstance(config["threshold"], (float, int)):
                    self.threshold = float(config["threshold"])
                    if self.verbose:
                        print(f"已加载阈值配置: {self.threshold:.2f}")
        except Exception as e:
            print(f"加载阈值配置失败: {e}")
    
    def load_model(self):
        """加载模型文件"""
        try:
            with open(self.model_path, 'r', encoding='utf-8') as f:
                self.model_data = json.load(f)
            if self.verbose:
                print(f"成功加载朴素贝叶斯模型: {self.model_path}")
        except Exception as e:
            print(f"加载朴素贝叶斯模型失败: {e}")
            self.model_data = None
    
    def get_ast_words(self, filepath: str) -> List[str]:
        """
        从PHP文件中提取AST词袋特征
        
        Args:
            filepath: PHP文件路径
            
        Returns:
            提取的词袋特征列表
        """
        if not os.path.exists(filepath):
            print(f"文件不存在: {filepath}")
            return []
        
        try:
            ast_data = self.ast_parser.get_file_ast(filepath)
            
            if ast_data and ast_data.get('status') == 'successed' and 'ast' in ast_data:
                ast_raw = ast_data['ast']
                ast_transformed = transform_ast_node_py(ast_raw)
                
                if ast_transformed:
                    words = extract_words_from_ast(ast_transformed, self.dangerous_funcs)
                    return words
                else:
                    print(f"AST变换失败: {filepath}")
            else:
                status = ast_data.get('status', 'N/A') if ast_data else 'N/A'
                reason = ast_data.get('reason', 'N/A') if ast_data else 'N/A'
                print(f"无法提取AST: {filepath}, 状态: {status}, 原因: {reason}")
        except Exception as e:
            print(f"处理文件出错: {filepath}, 错误: {e}")
            
        return []
    
    def get_normalized_score(self, words: List[str]) -> float:
        """
        计算词袋特征的WebShell得分
        
        Args:
            words: 词袋特征列表
            
        Returns:
            归一化的WebShell得分[0-1]
        """
        if not self.model_data or not words:
            return 0.0
            
        scores = {}
        total_docs = self.model_data.get("totalDocumentCount", 0)
        
        if total_docs == 0:
            return 0.0
        
        for class_name, class_data in self.model_data.items():
            if class_name == "totalDocumentCount":
                continue
                
            # 先验概率计算
            doc_count = class_data.get("docCount", 0)
            prior = doc_count / total_docs
            
            # 词汇表和总词数
            word_counts = class_data.get("wordCount", {})
            total_word_count = class_data.get("totalWordCount", 0)
            vocab_size = len(word_counts)
            
            # 计算对数后验概率
            import math
            posterior = math.log(prior)
            
            for word in words:
                # 获取当前词在该类别中的计数，如果不存在则为0
                word_count = word_counts.get(word, 0)
                
                # 使用拉普拉斯平滑计算条件概率
                word_prob = (word_count + 1) / (total_word_count + vocab_size)
                posterior += math.log(word_prob)
                
            scores[class_name] = posterior
        
        # 归一化概率
        if "webshell" not in scores or "normal" not in scores:
            return 0.0
            
        # 转换对数概率为普通概率并归一化
        max_score = max(scores.values())
        exp_scores = {cls: math.exp(score - max_score) for cls, score in scores.items()}
        total = sum(exp_scores.values())
        
        if total > 0:
            return exp_scores["webshell"] / total
        return 0.0
    
    def check_file(self, filepath: str) -> Dict[str, Any]:
        """
        检查单个PHP文件
        
        Args:
            filepath: 文件路径
            
        Returns:
            检测结果字典
        """
        start_time = time.time()
        
        # 提取特征
        words = self.get_ast_words(filepath)
        
        # 危险函数标记
        danger_funcs = [w[12:] for w in words if w.startswith("DANGER_FUNC_")]
        
        # 计算得分
        score = self.get_normalized_score(words)
        
        # 判断是否为WebShell
        is_webshell = score >= self.threshold
        
        # 处理时间
        duration = time.time() - start_time
        
        result = {
            "file": filepath,
            "score": score,
            "threshold": self.threshold,
            "is_webshell": is_webshell,
            "danger_funcs": danger_funcs,
            "duration_ms": duration * 1000,
            "verdict": "木马文件" if score >= 0.7 else 
                      "疑似木马" if score >= 0.5 else 
                      "低风险" if score >= 0.3 else "正常文件"
        }
        
        return result
    
    def check_directory(self, directory: str, recursive: bool = True) -> List[Dict[str, Any]]:
        """
        检查目录中的所有PHP文件
        
        Args:
            directory: 目录路径
            recursive: 是否递归检查子目录
            
        Returns:
            所有文件的检测结果列表
        """
        if not os.path.isdir(directory):
            print(f"目录不存在: {directory}")
            return []
        
        results = []
        
        if recursive:
            # 递归遍历
            for root, _, files in os.walk(directory):
                for filename in files:
                    if filename.lower().endswith('.php'):
                        filepath = os.path.join(root, filename)
                        result = self.check_file(filepath)
                        results.append(result)
        else:
            # 仅检查当前目录
            for filename in os.listdir(directory):
                if filename.lower().endswith('.php'):
                    filepath = os.path.join(directory, filename)
                    if os.path.isfile(filepath):
                        result = self.check_file(filepath)
                        results.append(result)
        
        return results

def print_result(result: Dict[str, Any], verbose: bool = False):
    """格式化输出检测结果"""
    filename = os.path.basename(result["file"])
    score = result["score"]
    verdict = result["verdict"]
    
    if verbose:
        # 详细模式
        # 只输出疑似木马和木马的信息
        if score < 0.6:
            return
        danger_funcs = ", ".join(result["danger_funcs"]) if result["danger_funcs"] else "无"
        print(f"文件: {filename}")
        print(f"  路径: {result['file']}")
        print(f"  得分: {score:.4f} (阈值: {result['threshold']:.2f})")
        print(f"  判定: {verdict}")
        print(f"  危险函数: {danger_funcs}")
        print(f"  处理时间: {result['duration_ms']:.2f}ms")
        print()
    else:
        # 简洁模式
        risk_icon = "❌" if score >= 0.7 else "⚠️" if score >= 0.5 else "✅"
        danger_funcs = f" [{', '.join(result['danger_funcs'])}]" if result["danger_funcs"] else ""
        print(f"{risk_icon} {filename:<30} {score:.4f} {verdict}{danger_funcs}")

def main():
    parser = argparse.ArgumentParser(description="使用朴素贝叶斯模型检测PHP文件是否为WebShell")
    parser.add_argument("--model", default="data/models/Words.model", help="模型文件路径")
    parser.add_argument("--threshold", help="自定义阈值配置文件路径")
    parser.add_argument("--path", required=True, help="要检测的PHP文件或目录")
    parser.add_argument("--recursive", "-r", action="store_true", help="递归检查子目录")
    parser.add_argument("--verbose", "-v", action="store_true", help="输出详细信息")
    parser.add_argument("--output", "-o", help="输出结果到JSON文件")
    parser.add_argument("--php-version", default="7", choices=["5", "7", "8"], help="PHP版本")
    args = parser.parse_args()
    
    # 初始化检测器
    checker = BayesModelChecker(args.model, args.threshold, args.verbose)
    
    target_path = args.path
    results = []
    
    # 根据目标类型进行检测
    if os.path.isfile(target_path):
        # 单个文件检测
        result = checker.check_file(target_path)
        results.append(result)
    elif os.path.isdir(target_path):
        # 目录检测
        results = checker.check_directory(target_path, args.recursive)
    else:
        print(f"目标路径不存在: {target_path}")
        return
    
    # 按得分排序
    results.sort(key=lambda x: x["score"], reverse=True)
    
    # 输出结果
    print(f"\n检测结果 ({len(results)} 个文件):")
    print("=" * 60)
    
    for result in results:
        print_result(result, args.verbose)
    
    # 统计结果
    webshell_count = sum(1 for r in results if r["is_webshell"])
    suspect_count = sum(1 for r in results if 0.5 <= r["score"] < 0.7)
    
    print("=" * 60)
    print(f"检测总结: 共 {len(results)} 个文件, {webshell_count} 个木马, {suspect_count} 个可疑文件")
    
    # 保存结果到JSON文件
    if args.output:
        try:
            with open(args.output, 'w', encoding='utf-8') as f:
                json.dump({
                    "results": results,
                    "summary": {
                        "total": len(results),
                        "webshell": webshell_count,
                        "suspect": suspect_count
                    }
                }, f, ensure_ascii=False, indent=2)
            print(f"结果已保存到: {args.output}")
        except Exception as e:
            print(f"保存结果失败: {e}")

if __name__ == "__main__":
    main()