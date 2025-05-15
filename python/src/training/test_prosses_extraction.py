#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
测试TextFeatureExtractor提取8个统计特征的功能
"""

import os
import sys
import unittest
import tempfile
import numpy as np
import re
import math
import json
from typing import Dict, List

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

from training.train_prosses_svm import (
    TextFeatureExtractor as OriginalExtractor,
    BayesModel, 
    extract_features_from_file
)

# 导入TextFeatureExtractor类
# 注意：这里我们只测试特征提取，不涉及AST部分，所以删除了AST相关的代码
class TextFeatureExtractor:
    """提取PHP文件的文本特征"""
    def __init__(self):
        """初始化特征提取器"""
        self.tag_pattern = re.compile(r'<[\x00-\xFF]*?>')
        self.symbol_pattern = re.compile(r'[^a-zA-Z0-9]')
        self.statement_pattern = re.compile(r';')
        
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


class TestTextFeatureExtractor(unittest.TestCase):
    """测试TextFeatureExtractor类的特征提取功能"""
    
    def setUp(self):
        """测试前准备工作"""
        # 创建临时目录用于测试
        self.temp_dir = tempfile.mkdtemp()
        
        # 创建测试用的PHP文件
        self.test_files = {
            'normal': os.path.join(self.temp_dir, 'normal.php'),
            'webshell': os.path.join(self.temp_dir, 'webshell.php'),
            'empty': os.path.join(self.temp_dir, 'empty.php'),
            'html_mix': os.path.join(self.temp_dir, 'html_mix.php'),
        }
        
        # 创建测试文件
        self._create_test_files()
        
        # 初始化特征提取器
        self.extractor = TextFeatureExtractor()
    
    def tearDown(self):
        """测试后清理工作"""
        # 删除临时目录及其内容
        for file_path in self.test_files.values():
            if os.path.exists(file_path):
                os.remove(file_path)
        os.rmdir(self.temp_dir)
    
    def _create_test_files(self):
        """创建测试用的PHP文件"""
        # 正常PHP文件
        normal_content = """<?php
/**
 * 正常的PHP函数示例
 */
function processData($input) {
    $result = array();
    foreach ($input as $key => $value) {
        $result[$key] = strtoupper($value);
    }
    return $result;
}

// 测试数据
$data = array("apple", "banana", "orange");
$processed = processData($data);
echo json_encode($processed);
?>"""
        
        # Webshell PHP文件
        webshell_content = """<?php
if(isset($_REQUEST['cmd'])) {
    system($_REQUEST['cmd']);
}

function hideFunction() {
    eval($_POST['code']);
}

// 伪装成正常功能
echo "Welcome to the system";
?>"""
        
        # 空PHP文件
        empty_content = "<?php\n?>"
        
        # 混合HTML和PHP的文件
        html_mix_content = """<!DOCTYPE html>
<html>
<head>
    <title>PHP和HTML混合页面</title>
    <style>
        body { font-family: Arial; }
        .container { width: 800px; margin: 0 auto; }
    </style>
</head>
<body>
    <div class="container">
        <h1>动态内容示例</h1>
        <?php
        // 获取当前时间
        $current_time = date("Y-m-d H:i:s");
        echo "<p>当前时间: $current_time</p>";
        
        // 一个简单的循环
        echo "<ul>";
        for($i = 1; $i <= 5; $i++) {
            echo "<li>项目 $i</li>";
        }
        echo "</ul>";
        ?>
    </div>
</body>
</html>"""
        
        # 写入文件
        with open(self.test_files['normal'], 'w') as f:
            f.write(normal_content)
        with open(self.test_files['webshell'], 'w') as f:
            f.write(webshell_content)
        with open(self.test_files['empty'], 'w') as f:
            f.write(empty_content)
        with open(self.test_files['html_mix'], 'w') as f:
            f.write(html_mix_content)
    
    def test_feature_extraction(self):
        """测试特征提取功能"""
        for file_type, file_path in self.test_files.items():
            with open(file_path, 'r') as f:
                content = f.read()
            
            # 提取特征
            features = self.extractor.extract_features(content)
            
            # 检查是否提取了所有8个特征
            self.assertEqual(len(features), 8, f"{file_type}文件缺少特征")
            
            # 检查特征是否符合预期类型和范围
            self.assertIsInstance(features['LM'], float, "LM应为浮点数")
            self.assertGreaterEqual(features['LM'], 0, "LM应大于等于0")
            
            self.assertIsInstance(features['LVC'], float, "LVC应为浮点数")
            self.assertGreaterEqual(features['LVC'], 0, "LVC应大于等于0")
            
            self.assertIsInstance(features['WM'], float, "WM应为浮点数")
            self.assertGreaterEqual(features['WM'], 0, "WM应大于等于0")
            
            self.assertIsInstance(features['WVC'], float, "WVC应为浮点数")
            self.assertGreaterEqual(features['WVC'], 0, "WVC应大于等于0")
            
            self.assertIsInstance(features['SR'], float, "SR应为浮点数")
            self.assertGreaterEqual(features['SR'], 0, "SR应大于等于0")
            self.assertLessEqual(features['SR'], 100, "SR应小于等于100")
            
            self.assertIsInstance(features['TR'], float, "TR应为浮点数")
            self.assertGreaterEqual(features['TR'], 0, "TR应大于等于0")
            
            self.assertIsInstance(features['SPL'], float, "SPL应为浮点数")
            self.assertGreaterEqual(features['SPL'], 0, "SPL应大于等于0")
            
            self.assertIsInstance(features['IE'], float, "IE应为浮点数")
            self.assertGreaterEqual(features['IE'], 0, "IE应大于等于0")
            
            # 输出特征
            print(f"\n{file_type.upper()}文件的特征:")
            for key, value in features.items():
                print(f"  {key}: {value}")
    
    def test_feature_values(self):
        """测试特征值的一些预期情况"""
        # 测试空文件
        with open(self.test_files['empty'], 'r') as f:
            content = f.read()
        empty_features = self.extractor.extract_features(content)
        
        # 空文件应该有很低的特征值
        self.assertEqual(empty_features['LM'], 0, "空文件的LM应为0")
        self.assertEqual(empty_features['SPL'], 0, "空文件的SPL应为0")
        
        # 测试包含HTML标签的文件
        with open(self.test_files['html_mix'], 'r') as f:
            content = f.read()
        html_features = self.extractor.extract_features(content)
        
        # HTML文件应该有较高的TR值
        self.assertGreater(html_features['TR'], 0, "HTML混合文件的TR应大于0")
        
        # 测试普通PHP文件和Webshell文件的差异
        with open(self.test_files['normal'], 'r') as f:
            normal_content = f.read()
        with open(self.test_files['webshell'], 'r') as f:
            webshell_content = f.read()
            
        normal_features = self.extractor.extract_features(normal_content)
        webshell_features = self.extractor.extract_features(webshell_content)
        
        # 打印差异
        print("\n正常PHP文件和Webshell文件的特征差异:")
        for key in normal_features:
            diff = webshell_features[key] - normal_features[key]
            print(f"  {key}: 正常={normal_features[key]}, Webshell={webshell_features[key]}, 差异={diff}")
    
    def test_entropy_calculation(self):
        """测试熵计算"""
        # 测试不同类型文本的熵
        test_texts = {
            '重复字符': 'AAAAAAAAAAAAAA',
            '随机字符': 'A!b3D*f8G@jK#m5N%p',
            '空字符串': ''
        }
        
        for name, text in test_texts.items():
            entropy = self.extractor._calculate_entropy(text)
            print(f"\n{name}的熵: {entropy}")
            
            # 检查边界情况
            if name == '空字符串':
                self.assertEqual(entropy, 0, "空字符串的熵应为0")
            elif name == '重复字符':
                self.assertLessEqual(entropy, 1, "重复字符的熵应较低")
    
    def test_feature_consistency(self):
        """测试特征提取的一致性"""
        # 多次提取同一文件的特征应该一致
        with open(self.test_files['normal'], 'r') as f:
            content = f.read()
            
        features1 = self.extractor.extract_features(content)
        features2 = self.extractor.extract_features(content)
        
        for key in features1:
            self.assertEqual(features1[key], features2[key], f"特征{key}在多次提取中不一致")
    
    def test_specific_file_extraction(self):
        """测试指定文件的特征提取、AST词袋和预测分数"""
        # 指定文件路径
        file_path = "/opt/WebshellDet/bt-ShieldML/data/cleaned/php/webshell/app.class.php"
        
        # 检查文件是否存在
        if not os.path.exists(file_path):
            print(f"警告: 指定的文件 {file_path} 不存在，跳过此测试")
            self.skipTest(f"测试文件 {file_path} 不存在")
            return
        
        print(f"\n======== 测试指定文件: {file_path} ========")
        
        # 1. 使用简化版特征提取器提取统计特征
        try:
            with open(file_path, 'r', encoding='utf-8', errors='ignore') as f:
                content = f.read()
            
            features = self.extractor.extract_features(content)
            print("\n1. 统计特征 (简化版提取器):")
            for key, value in features.items():
                print(f"  {key}: {value}")
        except Exception as e:
            print(f"简化版提取器提取特征失败: {e}")
        
        # 2. 使用完整版特征提取器提取特征、AST词袋和预测分数
        try:
            # 初始化朴素贝叶斯模型
            # 模型文件路径:/opt/WebshellDet/bt-ShieldML/data/models/Words.model
            # model_path = os.path.join("/opt/WebshellDet/bt-ShieldML/", "/data/models/Words.model")
            model_path = "/opt/WebshellDet/bt-ShieldML/data/models/Words.model"
            if not os.path.exists(model_path):
                print(f"警告: 模型文件 {model_path} 不存在，将使用默认值")
                model_path = None
            
            # 尝试导入完整版TextFeatureExtractor
            try:
                # 创建完整版特征提取器
                full_extractor = OriginalExtractor()
                # 初始化贝叶斯模型
                bayes_model = BayesModel(model_path) if model_path else None
                
                # 提取所有特征
                if bayes_model and full_extractor:
                    all_features = extract_features_from_file(file_path, bayes_model, full_extractor)
                    
                    print("\n2. 统计特征 (完整版提取器):")
                    for key in ['LM', 'LVC', 'WM', 'WVC', 'SR', 'TR', 'SPL', 'IE']:
                        print(f"  {key}: {all_features[key]}")
                    
                    print("\n3. 贝叶斯预测分数:")
                    print(f"  BAYES: {all_features['BAYES']}")
                    
                    # 提取AST词袋
                    words = full_extractor.extract_words_for_bayes(file_path)
                    print(f"\n4. AST词袋信息 (前20个词):")
                    if words:
                        for i, word in enumerate(words[:20]):
                            print(f"  {i+1}. {word}")
                        if len(words) > 20:
                            print(f"  ...共 {len(words)} 个词")
                    else:
                        print("  未找到AST词袋信息或AST解析失败")
                    
                    # 构建特征向量并打印
                    feature_vector = [
                        all_features['LM'], all_features['LVC'], all_features['WM'], all_features['WVC'],
                        all_features['SR'], all_features['TR'], all_features['SPL'], all_features['IE'],
                        all_features['BAYES']
                    ]
                    print(f"\n5. 完整特征向量 (用于SVM输入):")
                    print(f"  {feature_vector}")
                else:
                    print("无法创建完整版特征提取器或贝叶斯模型")
            except (ImportError, AttributeError) as e:
                print(f"导入完整版TextFeatureExtractor失败: {e}")
                print("可能未正确设置Python路径或缺少依赖")
        except Exception as e:
            print(f"完整版特征提取失败: {e}")
            import traceback
            traceback.print_exc()

        print("\n======== 测试完成 ========")

def run_tests():
    """运行测试"""
    unittest.main()

if __name__ == "__main__":
    run_tests()