# 第一阶段：构建环境
FROM golang:1.22-bullseye AS builder

WORKDIR /build

# 安装系统依赖
RUN apt-get update && apt-get install -y \
    build-essential \
    automake \
    libtool \
    make \
    gcc \
    pkg-config \
    libssl-dev \
    libmagic-dev \
    jansson-dev \
    xxd \
    wget \
    && rm -rf /var/lib/apt/lists/*

# 安装YARA 4.3+
RUN wget https://github.com/VirusTotal/yara/archive/refs/tags/v4.3.1.tar.gz \
    && tar -xzf v4.3.1.tar.gz \
    && cd yara-4.3.1 \
    && ./bootstrap.sh \
    && ./configure --enable-static \
    && make \
    && make install \
    && ldconfig

# 复制源代码
COPY . .

# 安装Go依赖
RUN go mod download \
    && go get github.com/CyrusF/libsvm-go \
    && go get github.com/CyrusF/go-bayesian \
    && go get github.com/grd/stat

# 编译PHP桥接
RUN make -C php-bridge

# 执行构建脚本
RUN bash build.sh

# 第二阶段：运行环境
FROM debian:11-slim

# 设置工作目录
WORKDIR /www/dk_project/dk_app/shieldml/

# 安装必要的运行时依赖
RUN apt-get update && apt-get install -y \
    ca-certificates \
    tzdata \
    wget \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /www/dk_project/dk_app/shieldml/data

# 设置时区为亚洲/上海
ENV TZ=Asia/Shanghai

# 复制必要文件到容器中
COPY shieldml_server /www/dk_project/dk_app/shieldml/
COPY bt-shieldml /www/dk_project/dk_app/shieldml/
COPY shieldml_scan.html /www/dk_project/dk_app/shieldml/

# 确保文件有执行权限
RUN chmod +x /www/dk_project/dk_app/shieldml/shieldml_server && \
    chmod +x /www/dk_project/dk_app/shieldml/bt-shieldml && \
    echo '{"results":[]}' > /www/dk_project/dk_app/shieldml/data/webshellJson.json && \
    chmod 777 /www/dk_project/dk_app/shieldml/data/webshellJson.json && \
    chmod 777 /www/dk_project/dk_app/shieldml/data

# 暴露端口
EXPOSE 6528

# 健康检查
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget -qO- http://localhost:6528/shieldml_scan.html || exit 1

# 启动服务
CMD ["/www/dk_project/dk_app/shieldml/shieldml_server"]
