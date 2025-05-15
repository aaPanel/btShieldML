'''
Date: 2025-04-18 09:34:23
Editors: Mr wpl
Description: 
'''
# 引用 php_ast 模块 ast_parser_wrapper的get_file_ast
from ast_parser_wrapper import php_ast
from collections import deque
import json
import os
import subprocess
import re
from typing import Optional, Union, List, Any, Dict, Tuple
from collections import deque, namedtuple


def extract_keys_bfs(ast_root):
    """
    使用广度优先搜索 (BFS) 从 AST 结构中提取所有 'kind' 的值。

    Args:
        ast_root: AST 的根节点（通常是一个字典或列表）。

    Returns:
        一个包含所有找到的 'kind' 值的列表。
    """
    if not isinstance(ast_root, (dict, list)):
        print("Error: AST root must be a dictionary or list.")
        return []

    kind_values = []
    queue = deque([ast_root]) # 初始化队列，放入根节点

    while queue:
        current_node = queue.popleft() # 取出队首节点

        # 1. 处理字典类型的节点
        if isinstance(current_node, dict):
            # 提取 kind 值（如果存在）
            kind = current_node.get('kind') # 使用 .get() 避免 KeyError
            if kind is not None:
                # 这里的 kind 值可能是数字（如 php-ast 扩展返回的）或字符串（如示例所示）
                # 我们直接添加获取到的值
                kind_values.append(kind)
            
            # 将子节点（字典或列表类型）加入队列
            # 需要遍历字典的所有值，因为子节点可能不在 'children' 键下
            for key, value in current_node.items():
                if isinstance(value, dict):
                    queue.append(value) # 将子字典加入队列
                elif isinstance(value, list):
                    # 如果值是列表，则遍历列表中的项
                    for item in value:
                        if isinstance(item, (dict, list)): # 只将字典或列表项加入队列
                            queue.append(item)

        # 2. 处理列表类型的节点（例如 AST_STMT_LIST 的 children）
        elif isinstance(current_node, list):
            for item in current_node:
                if isinstance(item, (dict, list)): # 只将字典或列表项加入队列
                    queue.append(item)

    return kind_values

# 定义队列节点的结构，模仿 Go 的 opQueueNode
QueueNode = namedtuple("QueueNode", ["key", "value", "layer", "father_node"])

def get_op_serial_python(ast_root: Optional[Union[Dict, List]]) -> List[List[Any]]:
    """
    使用广度优先搜索 (BFS) 从 AST 结构中提取操作序列 (kind 值)，
    并在每个序列前添加父节点的 kind 值，模仿 Go 的 getOpSerial 函数。

    Args:
        ast_root: AST 的根节点（通常是一个字典或列表）。

    Returns:
        一个包含多个操作序列的列表，每个序列是父 kind 值加上子 kind 值列表。
        kind 值保持为原始类型（通常是字符串）。
    """
    if not isinstance(ast_root, (dict, list)):
        print("Error: AST root must be a dictionary or list.")
        return []

    result_serials: List[List[Any]] = []
    now_serial: List[Any] = []
    # 注意：father_node 存储的是父节点的字典表示，而不是特定的 astNode 对象
    queue = deque([QueueNode(key="root", value=ast_root, layer=0, father_node=None)])

    while queue:
        node = queue.popleft()
        current_value = node.value
        current_key = node.key
        current_layer = node.layer
        current_father = node.father_node # 这是父节点的字典

        # --- 处理不同类型的节点 ---
        if isinstance(current_value, dict):
            # 这是一个 AST 节点字典
            node_kind = current_value.get('kind')
            if node_kind is not None:
                now_serial.append(node_kind) # 添加当前节点的 kind

            # 将子节点加入队列，传递当前节点字典作为父节点
            children = current_value.get('children')
            if children is not None:
                queue.append(QueueNode(key="children", value=children, layer=current_layer + 1, father_node=current_value))
            # Go 代码还会遍历其他 key，这里我们也这样做以防万一
            # (但在 php-ast 中，子节点通常都在 'children' 下)
            # for k, v in current_value.items():
            #     if k != 'children' and isinstance(v, (dict, list)):
            #          queue.append(QueueNode(key=k, value=v, layer=current_layer + 1, father_node=current_value))


        elif isinstance(current_value, list):
            # 这是一个子节点列表
            needs_separator = (current_key == "children") # 检查是否需要分隔符

            # 将列表中的每个元素加入队列
            for i, item in enumerate(current_value):
                # 只有字典或列表类型的子项才被视为需要进一步遍历的节点
                if isinstance(item, (dict, list)):
                    queue.append(QueueNode(key=str(i), value=item, layer=current_layer + 1, father_node=current_father))
                # Go 代码似乎也处理非节点类型？这里我们保持一致，但可能不需要
                # else:
                #     queue.append(QueueNode(key=str(i), value=item, layer=current_layer + 1, father_node=current_father))

            # 如果需要，在处理完列表所有元素后添加分隔符
            if needs_separator:
                queue.append(QueueNode(key="separator", value=None, layer=current_layer + 1, father_node=current_father))

        elif current_value is None:
            # 遇到分隔符或其他 None 值
            if current_key == "separator":
                if now_serial: # 只有当前序列不为空时才处理
                    # 尝试获取父节点的 kind
                    father_kind = "UNKNOWN_FATHER" # 默认值
                    if isinstance(current_father, dict): # 确保父节点是字典
                         father_kind = current_father.get('kind', 'UNKNOWN_FATHER') # 获取 kind，或使用默认

                    # 构建最终序列：父 kind + 当前序列
                    finish_serial = [father_kind] + now_serial
                    result_serials.append(finish_serial)
                    now_serial = [] # 重置当前序列

        # 其他类型 (string, float/int) 在 BFS 遍历中通常作为叶子节点，
        # Go 代码中显式忽略了它们对 now_serial 的影响，这里也一样。
        # 它们可能会被放入队列（如果父节点是 list），但不会触发 kind 的添加。

    # 处理 BFS 结束后可能剩余的 now_serial (理论上不应该发生，因为总有根节点结束)
    # 但为了健壮性可以加上
    if now_serial:
        father_kind = "ROOT_OR_UNKNOWN"
        if isinstance(node.father_node, dict): # 使用最后一个处理节点的父节点
            father_kind = node.father_node.get('kind', 'ROOT_OR_UNKNOWN')
        finish_serial = [father_kind] + now_serial
        result_serials.append(finish_serial)


    return result_serials


def extract_words_from_ast(node: Any) -> List[str]:
    """
    递归地从 AST 节点中提取 'name' 字段作为词汇。
    模仿 Go 端 GetWordsAndCallable 的词汇提取部分。
    """
    words = []
    if isinstance(node, dict):
        # 提取当前节点的 'name' (如果存在且为字符串)
        if 'name' in node and isinstance(node['name'], str):
            words.append(node['name'])

        # 递归处理子节点 (通常在 'children' 下，但也可能在其他键下)
        for key, value in node.items():
            # 特别处理 children，因为它可能是列表或字典
            if key == 'children':
                if isinstance(value, dict):
                    # 如果 children 是字典，递归每个值
                    for child_key, child_value in value.items():
                        words.extend(extract_words_from_ast(child_value))
                elif isinstance(value, list):
                     # 如果 children 是列表，递归每个元素
                    for item in value:
                        words.extend(extract_words_from_ast(item))
            # 递归处理其他可能是节点或节点列表的值
            elif isinstance(value, dict):
                 words.extend(extract_words_from_ast(value))
            elif isinstance(value, list):
                 for item in value:
                     words.extend(extract_words_from_ast(item))

    elif isinstance(node, list):
        # 如果节点本身是列表，递归处理每个元素
        for item in node:
            words.extend(extract_words_from_ast(item))

    # 忽略基本类型如 string, int, float, bool, None
    return words

def extract_opcodes(filepath: str, php_executable: str = 'php') -> Optional[str]:
    """
    使用vld扩展从文件中提取PHP opcodes，适配详细输出格式。

    Args:
        filepath: PHP文件的路径。
        php_executable: PHP可执行文件的路径。

    Returns:
        如果成功，返回一个包含opcode的空格分隔字符串，否则返回None。
    """
    if not os.path.exists(filepath):
        print(f"文件未找到用于opcode提取: {filepath}")
        return None

    cmd = [
        php_executable,
        '-dvld.active=1',
        '-dvld.execute=0',
        filepath
    ]
    print(f"运行命令用于opcode提取: {' '.join(cmd)}")

    try:
        # --- 正确的提取逻辑，用于解析表格 ---
        output = subprocess.check_output(
            cmd, 
            stderr=subprocess.STDOUT
        )
        output_str = output.decode('utf-8', errors='ignore') # 使用 utf-8 解码，忽略可能的解码错误
       
        
        tokens = re.findall(r'\s(\b[A-Z_]+\b)\s', output_str)
        opcodes = " ".join(tokens)
        # --- 提取逻辑结束 ---

        if not opcodes:
             # 即使命令成功，也可能因为文件内容或VLD的特殊输出而没有Opcode
            return ""
        return opcodes

    except FileNotFoundError:
        print(f"'{php_executable}'命令未找到。无法提取opcode。")
        return ""
    except subprocess.TimeoutExpired:
        print(f"Opcode提取超时于{filepath}。跳过。")
        return ""
    except Exception as e:
        print(f"在{filepath}提取opcode时发生错误: {e}")
        return ""


# 初始化 php_ast 对象
php = php_ast()

# 获取文件的 AST
ast = php.get_file_ast("/opt/WebshellDet/bt-ShieldML/samples/php/check/huatailawfirm.com/vendor/wechatpay/phpqrcode/phpqrcode.php")

# 提取操作序列:提取key值
# 打印 AST
print(ast)

if ast and ast.get('status') == 'successed' and 'ast' in ast:
    ast_root_node = ast['ast']
    print(ast_root_node)
    ast_transformed = transform_ast_node_py(ast_root_node)

    print("提取的ast_transformed:", ast_transformed)
    words = extract_words_from_ast(ast_transformed)
    print("提取的词汇:", words)

else:
    print("无法从提供的示例数据中获取有效的 AST 根节点。")

