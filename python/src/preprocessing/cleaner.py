import os
import hashlib
import re
import subprocess
import argparse
import logging
import tempfile
from typing import Set, Tuple, Dict, List, Optional

# --- Configuration ---
# 清洗黑样本
# [1]去除掉重复的,空值,语法错误/无效的php文件
# [2]记录重复文件,无效文件,有效文件
# [3]写入clean_file.txt
# [4]控制台打印统计信息

# 白样本清洗
# [1]去除掉重复的,空值,语法错误/无效的php文件
# [2]记录重复文件,无效文件,有效文件
# [3]写入clean_file.txt
# [4]控制台打印统计信息

# 黑样本路径: /opt/WebshellDet/bt-ShieldML/samples/php/webshell
# 清洗后路径: /opt/WebshellDet/bt-ShieldML/data/cleaned/php/webshell
# 白样本路径: /opt/WebshellDet/bt-ShieldML/samples/php/normal
# 清洗后路径: /opt/WebshellDet/bt-ShieldML/data/cleaned/php/normal
# 清洗相关文件: clean_file.txt
# 使用
#     python3 python/src/preprocessing/cleaner.py --clean-webshell
#     python3 python/src/preprocessing/cleaner.py --clean-normal  
#     python3 python/src/preprocessing/cleaner.py 

# 目前情况:白样本数量1.5w,webshell样本3K
# 优化1：将白样本降低到4K，随机筛选
logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')
logger = logging.getLogger() # Get the root logger


def calculate_hash(content: bytes) -> str:
    """计算给定字节内容的SHA-256哈希值."""
    return hashlib.sha256(content).hexdigest()

def clean_php_whitespace_and_comments(code_str: str) -> str:
    """
    移除PHP代码中的注释和多余的空白。
    - 移除 // 单行注释
    - 移除 /* */ 多行注释
    - 移除行首和行尾的空白字符
    - 移除完全空白的行
    """
    logger.debug("开始应用额外的代码清洗 (注释/空白)...")
    # 1. 移除多行注释 /* ... */ (非贪婪匹配)
    cleaned_code = re.sub(r'/\*.*?\*/', '', code_str, flags=re.DOTALL)
    # 2. 移除单行注释 // ...
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
    logger.debug("额外的代码清洗完成。")
    return result

def is_valid_php(content_to_check: str, source_filepath_for_log: str, php_executable: str = 'php') -> bool:
    """
    检查给定的PHP代码字符串是否有效,使用`php -l`。
    Args:
        content_to_check: 要检查的PHP代码内容字符串 (清洗后或原始)
        source_filepath_for_log: 用于日志记录的原始文件路径
        php_executable: php可执行文件路径
    Returns:
        如果语法有效,返回True,否则返回False
    """
    # (保持不变, 包含详细日志)
    temp_filepath = None
    try:
        # 使用utf-8编码创建临时文件
        with tempfile.NamedTemporaryFile(mode='w', suffix='.php', delete=False, encoding='utf-8') as temp_file:
            temp_file.write(content_to_check)
            temp_filepath = temp_file.name
        logger.debug(f"创建临时文件用于语法检查: {temp_filepath}")
        try:
            result = subprocess.run(
                [php_executable, '-l', temp_filepath],
                capture_output=True, text=True, check=False, timeout=20, encoding='utf-8', errors='ignore' # 指定编码
            )
            logger.debug(f"PHP -l for {os.path.basename(source_filepath_for_log)} 退出码: {result.returncode}. Stdout: '{result.stdout.strip()}', Stderr: '{result.stderr.strip()}'")
            # 检查条件更严格：必须返回0且stdout包含"No syntax errors detected"
            if result.returncode != 0 or "No syntax errors detected" not in result.stdout:
                logger.warning(f"检测到无效的PHP语法 (或php -l失败) 于 '{source_filepath_for_log}'. 退出码: {result.returncode}, Stdout: '{result.stdout.strip()}', Stderr: '{result.stderr.strip()}'. 跳过.")
                return False
            logger.debug(f"PHP 语法有效于: {source_filepath_for_log}")
            return True
        except FileNotFoundError:
            logger.error(f"'{php_executable}'命令未找到. PHP语法验证跳过. 假设文件 '{source_filepath_for_log}' 有效.")
            return True # 如果php命令找不到，我们假设它是有效的，以避免丢弃文件
        except subprocess.TimeoutExpired:
            logger.warning(f"PHP语法检查超时于基于内容从 '{source_filepath_for_log}' 的临时文件. 跳过文件.")
            return False
        except Exception as e:
            logger.error(f"PHP语法检查执行期间失败于 '{source_filepath_for_log}': {e}")
            return False
        finally:
            if temp_filepath and os.path.exists(temp_filepath):
                logger.debug(f"删除临时文件: {temp_filepath}")
                os.remove(temp_filepath)
    except Exception as e:
        # 增加对临时文件写入失败的日志
        logger.error(f"创建/写入临时文件用于语法检查时发生错误 (source: '{source_filepath_for_log}'): {e}")
        # 确保即使写入失败也尝试删除临时文件（如果它被创建了）
        if temp_filepath and os.path.exists(temp_filepath):
             logger.debug(f"删除可能失败的临时文件: {temp_filepath}")
             os.remove(temp_filepath)
        return False


# 修改 process_directory 使其更通用
def process_directory(
    source_dir: str,
    dest_dir: str,
    label: str,
    php_executable: str = 'php',
    filter_prefix: Optional[str] = None,
    apply_extra_cleaning: bool = False # 新增参数控制是否应用额外清洗
) -> Tuple[int, Dict[str, List[str]], List[str], List[str]]:
    """
    处理一个PHP样本目录,根据选项清洗它们,并记录详细信息。

    Args:
        source_dir: 包含源PHP文件的根目录。
        dest_dir: 清洗后的文件将被移动到的目标目录。
        label: 用于日志记录的标签字符串。
        php_executable: PHP 可执行文件的路径。
        filter_prefix: 如果提供，则只处理内容以此字符串开头的文件。
        apply_extra_cleaning: 如果为 True，则应用 clean_php_whitespace_and_comments 函数。

    Returns:
        一个包含: (总文件数, 重复文件信息字典, 无效文件路径列表, 有效文件目标路径列表) 的元组。
        重复文件信息字典: {hash: [filepath1, filepath2, ...]}
    """
    logger.info(f"--- Processing {label} samples from: {source_dir} ---")
    if apply_extra_cleaning:
        logger.info("将对此目录应用额外的清洗（移除注释和多余空白）。")
    os.makedirs(dest_dir, exist_ok=True)

    total_files_found = 0
    hash_to_first_path: Dict[str, str] = {}
    duplicate_log_data: Dict[str, List[str]] = {}
    invalid_files: List[str] = []
    valid_files_dest_paths: List[str] = []

    for root, _, files in os.walk(source_dir):
        for filename in files:
            # 仅处理 .php 文件，忽略大小写
            if not filename.lower().endswith(".php"):
                continue

            total_files_found += 1
            source_filepath = os.path.join(root, filename)
            logger.debug(f"处理文件: {source_filepath}")

            # --- 读取文件和处理编码 ---
            raw_content: bytes | None = None
            content_str: str | None = None
            try:
                # 尝试 UTF-8
                with open(source_filepath, 'r', encoding='utf-8') as f:
                    content_str = f.read()
                # 检查文件是否为空
                if not content_str:
                    logger.warning(f"文件 {source_filepath} 为空. 跳过.")
                    invalid_files.append(source_filepath + " (Empty)")
                    continue
                raw_content = content_str.encode('utf-8') # 使用 utf-8 编码字节
            except UnicodeDecodeError:
                try:
                    # 尝试 latin-1 并忽略错误转换为 UTF-8
                    with open(source_filepath, 'r', encoding='latin-1') as f:
                        content_str = f.read()
                    if not content_str: # 再次检查是否为空
                         logger.warning(f"文件 {source_filepath} (latin-1) 为空. 跳过.")
                         invalid_files.append(source_filepath + " (Empty after latin-1)")
                         continue
                    # 将 latin-1 读取的字符串编码为 utf-8，忽略无法编码的字符
                    raw_content = content_str.encode('utf-8', errors='ignore')
                    # 再将处理过的 utf-8 字节解码回 utf-8 字符串，忽略错误
                    content_str = raw_content.decode('utf-8', errors='ignore')
                    logger.debug(f"文件 {source_filepath} 使用 latin-1 读取并转换为 UTF-8 (忽略错误).")
                except Exception as e_inner:
                    logger.warning(f"无法使用 UTF-8 或 latin-1 读取文件 {source_filepath}: {e_inner}. 跳过.")
                    invalid_files.append(source_filepath + " (Read Error)")
                    continue
            except Exception as e_outer:
                logger.warning(f"读取文件 {source_filepath} 时发生错误: {e_outer}. 跳过.")
                invalid_files.append(source_filepath + " (Read Error)")
                continue

            # 确保 content_str 和 raw_content 都有效
            if content_str is None or raw_content is None:
                logger.warning(f"文件 {source_filepath} 内容读取后无效. 跳过.")
                invalid_files.append(source_filepath + " (Read Invalid)")
                continue

            # --- 根据 filter_prefix 过滤 ---
            if filter_prefix and not content_str.startswith(filter_prefix):
                logger.debug(f"文件 {source_filepath} 内容不以 '{filter_prefix}' 开头. 跳过.")
                invalid_files.append(source_filepath + f" (Prefix Mismatch: expected '{filter_prefix}')")
                continue

            # --- 应用额外的代码清洗 (如果需要) ---
            if apply_extra_cleaning:
                final_content_str = clean_php_whitespace_and_comments(content_str)
            else:
                final_content_str = content_str # 使用原始（可能经过编码转换的）字符串
            # 确保清洗后内容不为空
            if not final_content_str.strip():
                 logger.info(f"文件 {source_filepath} 内容处理后变为空. 跳过.")
                 invalid_files.append(source_filepath + " (Empty after processing)")
                 continue

            # 将最终要处理的内容编码为 bytes 用于哈希和写入
            final_content_bytes = final_content_str.encode('utf-8')

            # --- 去重并记录重复信息 ---
            content_hash = calculate_hash(final_content_bytes)
            if content_hash in hash_to_first_path:
                logger.debug(f"重复内容哈希 {content_hash} 找到于 {source_filepath}. (首次出现于 {hash_to_first_path[content_hash]}). 跳过处理和移动.")
                if content_hash not in duplicate_log_data:
                    duplicate_log_data[content_hash] = [hash_to_first_path[content_hash]]
                duplicate_log_data[content_hash].append(source_filepath)
                continue

            # 记录首次出现的路径
            hash_to_first_path[content_hash] = source_filepath

            # --- 验证语法 ---
            # 使用最终要写入的内容进行验证
            if not is_valid_php(final_content_str, source_filepath, php_executable):
                invalid_files.append(source_filepath + " (Invalid Syntax)")
                # 从 hash_to_first_path 中移除，以防后续有相同哈希但路径不同的有效文件
                del hash_to_first_path[content_hash]
                continue

            # --- 写入文件 (只有首次出现且有效的才写入) ---
            dest_filename = os.path.basename(source_filepath)
            dest_filepath = os.path.join(dest_dir, dest_filename)
            counter = 0
            # 处理目标文件名冲突
            if os.path.exists(dest_filepath):
                base, ext = os.path.splitext(dest_filename)
                while os.path.exists(dest_filepath):
                    counter += 1
                    dest_filename = f"{base}_{counter}{ext}"
                    dest_filepath = os.path.join(dest_dir, dest_filename)
                logger.warning(f"目标文件名冲突于 {os.path.basename(source_filepath)}, 重命名为 {dest_filename}")
            try:
                with open(dest_filepath, 'wb') as f_dest: # 使用 'wb' 写入字节
                    f_dest.write(final_content_bytes)
                logger.info(f"清洗并保存 '{os.path.basename(source_filepath)}' 为 '{dest_filename}' 到 {dest_dir}")
                valid_files_dest_paths.append(dest_filepath) # 记录成功写入的目标路径
            except Exception as e:
                logger.error(f"写入清洗后的文件到 {dest_filepath} 失败: {e}")
                invalid_files.append(source_filepath + " (Write Error)")
                if os.path.exists(dest_filepath):
                    try:
                        os.remove(dest_filepath)
                    except OSError as oe:
                        logger.error(f"删除写入失败的文件 {dest_filepath} 时出错: {oe}")
                # 写入失败，从 hash_to_first_path 中移除
                if content_hash in hash_to_first_path: # 检查一下以防万一
                   del hash_to_first_path[content_hash]

    logger.info(f"--- 完成处理 {label} 样本 ---")
    return total_files_found, duplicate_log_data, invalid_files, valid_files_dest_paths

# --- Main Execution ---
if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="清洗PHP黑白样本, 去重, 验证语法并记录详情。")

    # 黑样本路径参数
    parser.add_argument(
        "--webshell-src",
        default="/opt/WebshellDet/bt-ShieldML/samples/php/webshell",
        help="黑样本源目录。"
    )
    parser.add_argument(
        "--webshell-dest",
        default="/opt/WebshellDet/bt-ShieldML/data/cleaned/php/webshell",
        help="清洗后的黑样本目标目录。"
    )

    # 白样本路径参数
    parser.add_argument(
        "--normal-src",
        default="/opt/WebshellDet/bt-ShieldML/samples/php/normal",
        help="白样本源目录。"
    )
    parser.add_argument(
        "--normal-dest",
        default="/opt/WebshellDet/bt-ShieldML/data/cleaned/php/normal",
        help="清洗后的白样本目标目录。"
    )

    # 选择清洗类型的标志
    parser.add_argument(
        "--clean-webshell",
        action="store_true",
        help="只清洗黑样本 (webshell)。如果未指定任何 --clean-* 标志，则默认两者都清洗。"
    )
    parser.add_argument(
        "--clean-normal",
        action="store_true",
        help="只清洗白样本 (normal)。如果未指定任何 --clean-* 标志，则默认两者都清洗。"
    )

    # PHP可执行文件路径
    parser.add_argument(
        "--php-executable",
        default="php",
        help="用于语法检查的 PHP 可执行文件路径。"
    )


    parser.add_argument(
        "-v", "--verbose",
        action="store_true",
        help="启用调试日志。"
    )

    args = parser.parse_args()

    if args.verbose:
        logger.setLevel(logging.DEBUG)
    else:
        # 如果不是 verbose，可以考虑将日志级别设置为 INFO 或更高
        logger.setLevel(logging.INFO)


    # --- 决定处理哪些样本 ---
    # 如果用户明确指定了至少一个类型，则只处理指定的类型
    # 否则（如果两个标志都为 False），则处理所有类型
    process_webshell = False
    process_normal = False

    # 检查是否有任何 --clean-* 标志被设置
    any_clean_flag_set = args.clean_webshell or args.clean_normal

    if any_clean_flag_set:
        process_webshell = args.clean_webshell
        process_normal = args.clean_normal
    else:
        # 默认情况：两者都处理
        process_webshell = True
        process_normal = True

    # --- 初始化统计数据 ---
    ws_total, ws_duplicate_data, ws_invalids, ws_valids_dest = 0, {}, [], []
    norm_total, norm_duplicate_data, norm_invalids, norm_valids_dest = 0, {}, [], []

    # --- 处理样本 ---
    if process_webshell:
        ws_total, ws_duplicate_data, ws_invalids, ws_valids_dest = process_directory(
            args.webshell_src, args.webshell_dest, "Webshell (Black)",
            php_executable=args.php_executable,
            filter_prefix=None,
            apply_extra_cleaning=False # 黑样本不应用额外清洗
        )

    if process_normal:
        norm_total, norm_duplicate_data, norm_invalids, norm_valids_dest = process_directory(
            args.normal_src, args.normal_dest, "Normal (White)",
            php_executable=args.php_executable,
            filter_prefix="<?php", # 白样本过滤前缀
            apply_extra_cleaning=True # 白样本应用额外清洗
        )

    # --- 计算统计数据 ---
    ws_inv_count = len(ws_invalids)
    ws_valid_count = len(ws_valids_dest)
    ws_dups_count = sum(len(paths) - 1 for paths in ws_duplicate_data.values())

    norm_inv_count = len(norm_invalids)
    norm_valid_count = len(norm_valids_dest)
    norm_dups_count = sum(len(paths) - 1 for paths in norm_duplicate_data.values())

    # --- 写入 clean_file.txt ---
    log_filename = "clean_file.txt" # 固定日志文件名
    try:
        logger.info(f"准备写入清洗详情到: {log_filename}")
        with open(log_filename, 'w', encoding='utf-8') as f_log:
            f_log.write("PHP 样本清洗详情\n")
            f_log.write("="*60 + "\n\n")

            # --- 黑样本部分 ---
            if process_webshell:
                f_log.write("黑样本 (Webshell) 清洗结果:\n")
                f_log.write("-" * 30 + "\n")
                f_log.write(f"源目录: {args.webshell_src}\n")
                f_log.write(f"目标目录: {args.webshell_dest}\n")
                f_log.write(f"额外清洗: 否 (保留原始代码结构)\n") # 说明黑样本不清洗
                f_log.write(f"总共扫描 PHP 文件数: {ws_total}\n")
                f_log.write(f"清洗后有效文件数: {ws_valid_count}\n")
                f_log.write(f"因无效/错误/空/不适用规则移除文件数: {ws_inv_count}\n")
                f_log.write(f"因内容重复移除文件数: {ws_dups_count}\n")
                f_log.write("-" * 30 + "\n\n")

                # 黑样本重复列表
                if ws_duplicate_data:
                    f_log.write("黑样本 - 重复文件列表:\n")
                    sorted_hashes = sorted(ws_duplicate_data.keys())
                    for content_hash in sorted_hashes:
                        paths = ws_duplicate_data[content_hash]
                        f_log.write(f"  哈希值: {content_hash}\n")
                        for path in paths: f_log.write(f"    - {path}\n")
                        f_log.write("\n")
                else:
                    f_log.write("黑样本 - 未检测到重复文件。\n\n")

                # 黑样本无效列表
                f_log.write("黑样本 - 无效/错误文件列表:\n")
                if ws_invalids:
                     for f_path_reason in sorted(ws_invalids): f_log.write(f"  - {f_path_reason}\n")
                else:
                     f_log.write("  (无)\n")
                f_log.write("\n" + "="*60 + "\n\n") # 添加分隔符

            # --- 白样本部分 ---
            if process_normal:
                f_log.write("白样本 (Normal) 清洗结果:\n")
                f_log.write("-" * 30 + "\n")
                f_log.write(f"源目录: {args.normal_src}\n")
                f_log.write(f"目标目录: {args.normal_dest}\n")
                f_log.write(f"过滤规则: 文件内容必须以 '<?php' 开头\n")
                f_log.write(f"额外清洗: 是 (移除注释和多余空白)\n") # 说明白样本应用了额外清洗
                f_log.write(f"总共扫描 PHP 文件数: {norm_total}\n")
                f_log.write(f"清洗后有效文件数: {norm_valid_count}\n")
                f_log.write(f"因无效/错误/空/不适用规则移除文件数: {norm_inv_count}\n")
                f_log.write(f"因内容重复移除文件数: {norm_dups_count}\n")
                f_log.write("-" * 30 + "\n\n")

                # 白样本重复列表
                if norm_duplicate_data:
                    f_log.write("白样本 - 重复文件列表:\n")
                    sorted_hashes = sorted(norm_duplicate_data.keys())
                    for content_hash in sorted_hashes:
                        paths = norm_duplicate_data[content_hash]
                        f_log.write(f"  哈希值: {content_hash}\n")
                        for path in paths: f_log.write(f"    - {path}\n")
                        f_log.write("\n")
                else:
                    f_log.write("白样本 - 未检测到重复文件。\n\n")

                 # 白样本无效列表
                f_log.write("白样本 - 无效/错误/过滤文件列表:\n")
                if norm_invalids:
                     for f_path_reason in sorted(norm_invalids): f_log.write(f"  - {f_path_reason}\n")
                else:
                     f_log.write("  (无)\n")
                f_log.write("\n" + "="*60 + "\n\n") # 添加分隔符

        logger.info(f"成功写入清洗详情到 {log_filename}")

    except Exception as e:
        logger.error(f"写入 {log_filename} 时发生错误: {e}")

    # --- 最终控制台报告 ---
    logger.info("\n" + "="*30 + " 清洗总结 (控制台) " + "="*30)
    if process_webshell:
        logger.info(f"黑样本 (Webshell):")
        logger.info(f"  源目录: {args.webshell_src} -> 目标目录: {args.webshell_dest}")
        logger.info(f"  额外清洗: 否") # 控制台也说明
        logger.info(f"  总扫描: {ws_total} | 有效写入: {ws_valid_count} | 重复移除: {ws_dups_count} | 无效/错误移除: {ws_inv_count}")
    if process_normal:
        logger.info(f"白样本 (Normal):")
        logger.info(f"  源目录: {args.normal_src} -> 目标目录: {args.normal_dest}")
        logger.info(f"  过滤规则: 内容以 '<?php' 开头")
        logger.info(f"  额外清洗: 是 (移除注释/空白)") # 控制台也说明
        logger.info(f"  总扫描: {norm_total} | 有效写入: {norm_valid_count} | 重复移除: {norm_dups_count} | 无效/错误/过滤移除: {norm_inv_count}")

    if not process_webshell and not process_normal:
         logger.info("未选择任何样本类型进行清洗。")
    else:
         logger.info(f"详细清洗日志已写入: {log_filename}")
    logger.info("="*78)