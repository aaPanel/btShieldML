'''
Date: 2025-04-16 14:50:30
Editors: Mr wpl
Description: 
'''
# coding: utf-8
# github: https://github.com/php-ast/php-ast
# author: php_ast
# version: 1.1
# time 2024-04-20

from ctypes import cdll, c_int, POINTER, c_void_p
import os, io
import time, sys, json

class php_ast:
    def __init__(self):
        self.__is_init = False
        self.stdin_r = None
        self.stdin_w = None
        self.stdout_r = None
        self.stdout_w = None

    def __enter__(self):
        """支持上下文管理器模式"""
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        """确保资源被正确释放"""
        self.cleanup()

    def php_init(self, version="7"):
        if self.__is_init:
            return
            
        # 为每个实例创建新的管道
        self.stdin_r, self.stdin_w = os.pipe()
        self.stdout_r, self.stdout_w = os.pipe()
        
        try:
            current_path = os.path.dirname(os.path.abspath(__file__))
            if version == "5" or version == "7":
                self.lib = cdll.LoadLibrary(current_path+"/libphp7.so")
            elif version == "8":
                self.lib = cdll.LoadLibrary(current_path+"/libphp8.so")
            else:
                raise Exception("Cannot find the php7 version")
                
            self.lib.init.argtypes = [c_void_p, c_void_p]
            self.lib.init.restype = c_int
            self.lib.execute.argtypes = []
            self.lib.execute.restype = c_int
            
            result = self.lib.init(self.stdin_r, self.stdout_w)
            if result != 0:
                raise Exception("Cannot initialize PHP runtime")
                
            result = self.lib.execute()
            if result != 0:
                raise Exception("Cannot start PHP runtime")
                
            self.__is_init = True
        except Exception as e:
            self.cleanup()
            raise e

    def get_ast(self, src,version="7"):
        if not self.__is_init:
            self.php_init(version)
            self.__is_init = True
        '''通过管道与PHP运行时通信'''
        os.write(self.stdin_w, f"{len(src)}\n".encode())
        os.write(self.stdin_w, src)
        # 读取响应长度
        data_len_bytes = b""
        while not data_len_bytes.endswith(b"\n"):
            chunk = os.read(self.stdout_r, 1)  # 逐字节读取
            if not chunk:
                break  # 如果读到 EOF，则退出
            data_len_bytes += chunk
        data_len_str = data_len_bytes.decode().strip()

        if not data_len_str.isdigit():
            return {"status": "success", "ast": {}}  # 返回空的AST
        data_len = int(data_len_str)
        data_bytes = b""
        while len(data_bytes) < data_len:
            data_bytes += os.read(self.stdout_r, 4096)
        data_str = data_bytes.decode().strip()
        # print(f"|---data_str: {data_str}")
        try:
            data = json.loads(data_str)
        except:
            data = {"status": "success", "ast": {}}
        return data


    def get_file_ast(self, file_path):
        '''通过文件获取AST'''
        if not os.path.exists(file_path):
            return {"status": "success", "ast": {}}
        f=open(file_path, "rb")
        src = f.read()
        f.close()
        return self.get_ast(src)

    # 追踪AST
    def trace_ast(self, ast):
        '''追踪AST'''
        if ast is None:
            return
        if type(ast) is not dict:
            return
        if 'kind' in ast:
            print("kind:", ast['kind'])

        for k, v in ast.items():
            if k == 'children' and type(v) is dict:
                for i in v:
                    self.trace_ast(v[i])
            elif k == 'children' and type(v) is list:
                for i in v:
                    self.trace_ast(i)
            else:
                self.trace_ast(v)

    def cleanup(self):
        """清理资源"""
        try:
            if self.__is_init:
                # 发送终止指令
                os.write(self.stdin_w, b"0\n")
                time.sleep(0.1)  # 等待PHP进程退出
                
                # 关闭所有管道
                for fd in [self.stdin_r, self.stdin_w, self.stdout_r, self.stdout_w]:
                    if fd is not None:
                        try:
                            os.close(fd)
                        except:
                            pass
                
                self.__is_init = False
        except Exception as e:
            print(f"清理PHP运行时出错: {e}")
