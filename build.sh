#!/bin/bash
###
 # @Date: 2025-04-18 12:03:27
 # @Editors: Mr wpl
 # @Description: 
### 
# 设置工作目录为项目根目录
cd "$(dirname "$0")"

# 创建嵌入文件的目录结构
mkdir -p pkg/embedded/data/models pkg/embedded/data/signatures

# 复制所有需要嵌入的文件到pkg/embedded目录下的相应位置
cp -f config.yaml pkg/embedded/
cp -f data/models/ProcessSVM.model.info pkg/embedded/data/models/
cp -f data/models/ProcessSVM.model.model pkg/embedded/data/models/
cp -f data/models/Words.model pkg/embedded/data/models/
cp -f data/signatures/Webshells_rules.yar pkg/embedded/data/signatures/

# 将yara静态库编译到go可执行文件中

# 第一步：确保 libyara.a 存在
# 告诉链接器：开始静态链接 (-Bstatic)，链接 yara 库 (-lyara)，然后切换回动态链接 (-Bdynamic) 后续库
export CGO_LDFLAGS="-Wl,-Bstatic -lyara -Wl,-Bdynamic"
# 第二步：运行构建命令
go build -tags yara_static -o bt-shieldml ./cmd/

echo "构建完成: bt-shieldml"