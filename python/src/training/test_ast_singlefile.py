'''
Date: 2025-05-06 16:32:18
Editors: Mr wpl
Description: 
'''
#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
测试AST解析器的简单脚本
"""
import os
import sys
import tempfile

# 添加src到路径
test_dir = os.path.dirname(os.path.abspath(__file__))
python_root_dir = os.path.dirname(os.path.dirname(test_dir))
src_dir = os.path.join(python_root_dir, 'src')
sys.path.insert(0, src_dir)

from preprocessing.ast_parser_wrapper import php_ast
from training.train_bayes import transform_ast_node_py, extract_words_from_ast

# 使用方法
# python test_ast_singlefile.py

def test_single_file_ast():
    """测试单个PHP文件的AST解析"""
    # 创建临时PHP文件
    with tempfile.NamedTemporaryFile(suffix='.php', mode='w', delete=False) as f:
        f.write("""<?php
        function test_function($param) {
            echo "Hello World!";
            return $param * 2;
        }
        $result = test_function(5);
        ?>""")
        temp_file = f.name
    
    try:
        # 创建AST解析器
        parser = php_ast()
        
        # 解析文件
        ast_data = parser.get_file_ast(temp_file)
        
        # 验证解析结果
        if ast_data.get('status') == 'successed' and 'ast' in ast_data:
            ast_raw = ast_data['ast']
            ast_transformed = transform_ast_node_py(ast_raw)
            words = extract_words_from_ast(ast_transformed)
            print(f"成功提取词汇: {words}")
            return True
        else:
            print(f"AST解析失败: {ast_data}")
            return False
    finally:
        # 清理资源
        if 'parser' in locals():
            parser.cleanup()
        
        # 删除临时文件
        if os.path.exists(temp_file):
            os.unlink(temp_file)

if __name__ == '__main__':
    if test_single_file_ast():
        print("单文件AST解析测试通过")
    else:
        print("单文件AST解析测试失败")