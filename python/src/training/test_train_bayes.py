'''
Date: 2025-04-22 21:35:58
Editors: Mr wpl
Description: 
'''
# path: python/tests/training/test_train_bayes.py
import unittest
import os
import sys
import json
import math
from typing import List, Dict, Any, Tuple


# 单元测试测试训练脚本,在根目录测试
# python3 -m unittest python/src/training/test_train_bayes.py

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
            self.model_data = None
    
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
            
        # 按照要求计算 webshell_score / (normal_score + webshell_score)
        webshell_score = scores["webshell"]
        # 这里直接返回webshell的概率值，因为predict已经做了归一化处理
        return webshell_score
    

class TestASTWordExtraction(unittest.TestCase):

    @classmethod
    def setUpClass(cls):
        """在所有测试开始前初始化 PHP AST 解析器"""
        cls.ast_parser = php_ast()
        # 可选：如果需要，显式初始化 PHP 环境
        # try:
        #     cls.ast_parser.php_init(version="7") # Or desired version
        # except Exception as e:
        #     raise RuntimeError(f"Failed to initialize PHP AST parser for tests: {e}") from e

    def test_extract_words_for_specific_file(self):
        """测试从指定 PHP 文件提取 AST 词袋信息"""
        # 黑样本
        # test_file_path = "/opt/WebshellDet/bt-ShieldML/data/cleaned/php/webshell/922a61bd758cffc0a56a079fa1843023d4ca1ffa_1.php"
        # test_file_path = "/opt/WebshellDet/bt-ShieldML/samples/php/check/huatailawfirm.com/vendor/wechatpay/lib/WxPayConfigInterface.php"
        test_file_path = "/opt/WebshellDet/bt-ShieldML/data/cleaned/php/webshell/1.php"
        # 正常文件
        # test_file_path ="/opt/WebshellDet/bt-ShieldML/data/cleaned/php/webshell/Crystal_shellb.php"
        # 1. 检查测试文件是否存在
        self.assertTrue(os.path.exists(test_file_path), f"测试文件不存在: {test_file_path}")

        # 2. 获取原始 AST
        ast_data = self.ast_parser.get_file_ast(test_file_path)
        self.assertIsNotNone(ast_data, "get_file_ast 返回了 None")
        self.assertEqual(ast_data.get('status'), 'successed', f"AST 解析失败: Status={ast_data.get('status')}, Reason={ast_data.get('reason')}")
        self.assertIn('ast', ast_data, "AST 结果中缺少 'ast' 键")
        ast_raw = ast_data['ast']
        # print("原始AST:", ast_raw)
        self.assertIsNotNone(ast_raw, "原始 AST 为 None") # 确保 ast 键对应的值不是 None

        # 3. 转换 AST 结构
        ast_transformed = transform_ast_node_py(ast_raw)
        # print("--------------------------------\n")
        # print("转换后的AST:", ast_transformed)
        self.assertIsNotNone(ast_transformed, "AST 转换后为 None")
        # 可以添加更具体的类型检查，例如检查根节点是否为 PyAstNode 或 list/dict
        self.assertTrue(isinstance(ast_transformed, (PyAstNode, list, dict)), f"转换后的 AST 类型不正确: {type(ast_transformed)}")

        # 4. 提取词袋
        extracted_words = extract_words_from_ast(ast_transformed)
        print(f"\n--- 从 {os.path.basename(test_file_path)} 提取的词袋 ---")
        print(extracted_words)
        print(f"--- 共 {len(extracted_words)} 个词 ---")

        # 5. 断言检查
        self.assertIsInstance(extracted_words, list, "提取的词袋不是列表类型")
        self.assertTrue(len(extracted_words) > 0, "提取的词袋为空，可能提取逻辑有误或该文件确实无 'name' 字段")

        # 可选：断言包含某些预期的关键词
        # 注意：这需要你手动分析目标 PHP 文件及其 AST 来确定预期词汇
        # 例如，如果知道该文件定义了函数 'my_function' 和变量 '$data'
        # expected_words = ['my_function', 'data']
        # for word in expected_words:
        #     self.assertIn(word, extracted_words, f"预期词汇 '{word}' 未在提取结果中找到")
        
    def test_bayes_model_prediction(self):
        """测试贝叶斯模型预测功能"""
        # 1. 加载贝叶斯模型
        model_path = "/opt/WebshellDet/bt-ShieldML/data/models/Words.model"
        if not os.path.exists(model_path):
            self.skipTest(f"贝叶斯模型文件不存在: {model_path}")
        
        bayes_model = BayesModel(model_path)
        self.assertIsNotNone(bayes_model.model_data, "贝叶斯模型加载失败")
        
        # 2. 测试正常PHP代码的预测
        normal_words = ["include", "function", "public", "return", "class", "string", "array", 
                        "null", "this", "extends", "namespace", "protected", "private"]
        normal_score = bayes_model.get_normalized_score(normal_words)
        print(f"\n正常PHP代码的WebShell概率: {normal_score:.6f}")
        self.assertLessEqual(normal_score, 0.5, "正常代码的WebShell概率应低于0.5")
        
        # 3. 测试WebShell代码的预测
        webshell_words = ["eval", "base64_decode", "system", "exec", "shell_exec", "passthru", 
                          "assert", "str_rot13", "gzinflate", "preg_replace", "create_function"]
        webshell_score = bayes_model.get_normalized_score(webshell_words)
        print(f"WebShell代码的WebShell概率: {webshell_score:.6f}")
        self.assertGreaterEqual(webshell_score, 0.5, "WebShell代码的WebShell概率应高于0.5")
        
        # 4. 测试从真实文件提取的词袋的预测
        # 测试黑样本
        black_file_path = "/opt/WebshellDet/bt-ShieldML/data/cleaned/php/webshell/1.php"
        if os.path.exists(black_file_path):
            ast_data = self.ast_parser.get_file_ast(black_file_path)
            if ast_data and ast_data.get('status') == 'successed' and 'ast' in ast_data:
                ast_transformed = transform_ast_node_py(ast_data['ast'])
                black_words = extract_words_from_ast(ast_transformed)
                black_score = bayes_model.get_normalized_score(black_words)
                print(f"黑样本文件的WebShell概率: {black_score:.6f}")
                print(f"黑样本文件提取的词袋: {black_words[:10]}... (显示前10个)")
                self.assertGreaterEqual(black_score, 0.5, "黑样本文件的WebShell概率应高于0.5")
        
        # 测试白样本
        white_file_path = "/opt/WebshellDet/bt-ShieldML/samples/php/check/huatailawfirm.com/login.php"
        if os.path.exists(white_file_path):
            ast_data = self.ast_parser.get_file_ast(white_file_path)
            if ast_data and ast_data.get('status') == 'successed' and 'ast' in ast_data:
                ast_transformed = transform_ast_node_py(ast_data['ast'])
                white_words = extract_words_from_ast(ast_transformed)
                white_score = bayes_model.get_normalized_score(white_words)
                print(f"白样本文件的WebShell概率: {white_score:.6f}")
                print(f"白样本文件提取的词袋: {white_words[:10]}... (显示前10个)")
                self.assertLessEqual(white_score, 0.5, "白样本文件的WebShell概率应低于0.5")


if __name__ == '__main__':
    unittest.main()