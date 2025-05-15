'''
Date: 2025-04-17 17:37:21
Editors: Mr wpl
Description: 测试清洗
'''
# 测试
import os
import hashlib
import re
import subprocess
import argparse
import logging
import tempfile
from typing import Set, Tuple, Dict, List, Optional


# 
def clean_php_whitespace_and_comments(code_str: str) -> str:
    """
    移除PHP代码中的注释和多余的空白。
    - 移除 //、# 单行注释
    - 移除 /* */ 多行注释
    - 移除行首和行尾的空白字符
    - 移除完全空白的行
    """
    print("开始应用额外的代码清洗 (注释/空白)...")
    # 1. 移除多行注释 /* ... */ (非贪婪匹配)
    cleaned_code = re.sub(r'/\*.*?\*/', '', code_str, flags=re.DOTALL)
    # 2. 移除单行注释 // ....
    cleaned_code = re.sub(r'//.*?$', '', cleaned_code, flags=re.MULTILINE)

    # 3. 处理行和行间空白
    lines = cleaned_code.splitlines()
    cleaned_lines = []
    for line in lines:
        stripped_line = line.strip() # 移除行首尾空白
        if stripped_line: # 只保留非空行
            cleaned_lines.append(stripped_line)

    # 4. 用单个换行符重新组合
    result = "\n".join(cleaned_lines)
    print("额外的代码清洗完成。")
    return result


# 测试
if __name__ == "__main__":
    # 测试数据,文件路径:/opt/WebshellDet/bt-ShieldML/samples/php/normal/maccms10-master/maccms10-master/thinkphp/library/think/template/TagLib.php
    file_path = "/opt/WebshellDet/bt-ShieldML/samples/php/normal/maccms10-master/maccms10-master/application/index/controller/Ajax.php"
    with open(file_path, "r", encoding="utf-8") as f:
        code_str = f.read()
    cleaned_code = clean_php_whitespace_and_comments(code_str)
    # 保存清洗后的代码到临时文件,文件名:cleaned_code.php
    # 保存路径:/opt/WebshellDet/bt-ShieldML/samples/php/normal/maccms10-master/maccms10-master/thinkphp/library/think/template/
    temp_dir = "/opt/WebshellDet/bt-ShieldML/samples/php/normal/maccms10-master/maccms10-master/thinkphp/library/think/template/"
    temp_file_path = os.path.join(temp_dir, "cleaned_code.php")
    with open(temp_file_path, "w", encoding="utf-8") as f:
        f.write(cleaned_code)
    print(f"清洗后的代码已保存到: {temp_file_path}")
    
    # php -l 测试
    php_cmd = "php -l " + temp_file_path
    result = subprocess.run(php_cmd, shell=True, check=True, capture_output=True, text=True)
    print(result.stdout)
    print(result.stderr)