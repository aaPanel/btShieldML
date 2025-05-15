<div align="center">
<img src="https://www.bt.cn/static/new/images/logo.svg" alt="btShieldML " width="300"/>
</div>

<h1 align="center">堡塔木马查杀引擎</h1>

<div align="center">

[![btShieldML](https://img.shields.io/badge/go-1.22-blue)](https://github.com/aaPanel/btShieldML)[![openresty](https://img.shields.io/badge/License-AGPLv3-blue)](https://github.com/aaPanel/btShieldML/blob/main/LICENSE)[![version](https://img.shields.io/github/v/release/aaPanel/btShieldML.svg?color=blue)](https://github.com/aaPanel/btShieldML)[![social](https://img.shields.io/github/stars/aaPanel/btShieldML?style=social)](https://github.com/aaPanel/btShieldML)

</div>

## 堡塔木马查杀引擎bt-ShieldML
> **免费的木马查杀引擎** bt-ShieldML是一款基于机器学习的堡塔木马查杀引擎，主要针对Web服务器环境中的恶意代码文件进行检测。 项目第一阶段聚焦于PHP文件的检测，后续将扩展支持更多语言，引擎将以Go语言实现，并编译为独立可执行文件，可集成到堡塔面板功能或作为独立工具使用。 **安装即可使用！**

公开内容
- 详细的模型训练过程
- GO程序核心源码

当前支持
- php文件检测
- 支持常见PHP webshell变种、混淆及加密技术识别
- 提供文件级别和目录级别的批量扫描

发展规划
- 将扩展支持ASP、JSP等多语言webshell检测
- 持续优化检测引擎，新增深度学习预测模型

## 功能介绍
###  堡塔木马查杀工作原理图
<p align="center">
    <img width="1986" alt="image" src="https://github.com/aaPanel/btShieldML/blob/main/img/Checking.png?raw=true">
</p>

### AST通信逻辑图

<p align="center">
    <img width="1986" alt="image" src="https://github.com/aaPanel/btShieldML/blob/main/img/ast.png?raw=true">
</p>

##  安装指南
### 第一种方法：直接下载编译好的二进制文件


### 第二种方法：编译源码
> 编译环境：Go 1.22 + Linux系统

第一步：安装依赖环境
```
go get github.com/CyrusF/libsvm-go
go get github.com/CyrusF/go-bayesian
go get github.com/grd/stat
apt install xxd
apt install libyara-dev
```

安装yara4.3+环境
```
# 在Debian系统安装YARA 4.3
wget https://github.com/VirusTotal/yara/archive/refs/tags/v4.3.1.tar.gz
tar -xzf v4.3.1.tar.gz
cd yara-4.3.1
./bootstrap.sh
./configure --enable-static
make
sudo make install

# 安装开发依赖
sudo apt-get install libssl-dev libmagic-dev jansson-dev

# 刷新动态库缓存
sudo ldconfig
```


第二步：进入根目录，编译php-bridge
```
cd bt-ShieldML && go mod tidy
make -C php-bridge
```

第三步：执行build.sh的脚本,需要把yara的静态库编译进去
```
bash build.sh
```


## 使用方法
基本命令行用法
```
./bt-shieldml -path /path/to/scan  # 终端输出
./bt-shieldml -path /path/to/scan -format json # 输出JSON格式文件，默认data目录下
./bt-shieldml -path /opt/WebshellDet/sample/webshell/tennc/PHP/ -output report.html  # 输出HTML格式文件
```


案例说明
```
测试1: 目录测试
./bt-shieldml -path  /opt/WebshellDet/sample/webshell/tennc/PHP/

测试2:单文件测试
./bt-shieldml -path  /opt/WebshellDet/sample/webshell/tennc/PHP/php/b374k/b374k-2.3.min.php

测试3:输出html报告
./bt-shieldml -path /opt/WebshellDet/sample/webshell/tennc/PHP/ -output report.html
```

<p align="center">
    <img width="1986" alt="image" src="https://github.com/aaPanel/btShieldML/blob/main/img/report.png?raw=true">
</p>

## web检测平台编译
> 默认是6528端口，可支持修改

编译并运行
```
go build -o webshell_server webshell_server.go
./webshell_server
```
访问 http://服务器ip:6528/webshell_scan.html 即可
<p align="center">
    <img width="1986" alt="image" src="https://github.com/aaPanel/btShieldML/blob/main/img/webserver.png?raw=true">
</p>


## 在线演示(Demo)
敬请期待……

## 许可证信息
该项目是开源的，可根据AGPLv3协议使用。

## 联系我们
>1. GitHub Issue 
>2. QQ群1：922160183   QQ群2：709033027


## 星趋势
> 欢迎关注我们的项目，我们会持续更新和优化项目。

[![Star History Chart](https://api.star-history.com/svg?repos=aaPanel/btShieldML&type=Date)](https://www.star-history.com/#aaPanel/btShieldML&Date)

