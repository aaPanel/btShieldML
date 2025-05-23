#!/bin/bash
# 设置目标架构（由环境变量传入，默认为amd64）
ARCH=${GOARCH:-amd64}

# 根据架构设置编译参数
if [ "$ARCH" = "arm64" ]; then
  export GOARCH=arm64
else
  export GOARCH=amd64
fi

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

# 设置完全静态编译的环境变量
export CGO_ENABLED=1
export GOOS=linux
export GOARCH=amd64
# 强制所有库静态链接
export CGO_LDFLAGS="-static"
# 禁用PIE（Position Independent Executable）
export CGO_CFLAGS="-O2 -g"

# 完全静态构建
go build -tags yara_static,netgo,osusergo -ldflags '-s -w -extldflags "-static"' -o bt-shieldml ./cmd/
echo "静态构建完成: bt-shieldml"

# 编译web服务平台
go build -tags netgo,osusergo -ldflags '-s -w -extldflags "-static"' -o shieldml_server ./shieldml_server.go

echo "静态构建完成: shieldml_server"