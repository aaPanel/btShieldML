# 第一阶段：构建环境
FROM golang:1.22-bullseye AS builder

WORKDIR /build

# 安装系统依赖
RUN apt-get update && apt-get install -y \
    build-essential \
    pkg-config \
    && rm -rf /var/lib/apt/lists/*

# 复制源代码
COPY . .

# 安装Go依赖
RUN go mod download

# 只编译shieldml_server
RUN go build -tags netgo,osusergo -ldflags '-s -w -extldflags "-static"' -o shieldml_server ./shieldml_server.go

# 第二阶段：运行环境
FROM debian:11-slim

WORKDIR /www/dk_project/dk_app/shieldml/

# 安装必要的运行时依赖
RUN apt-get update && apt-get install -y \
    ca-certificates \
    tzdata \
    wget \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /www/dk_project/dk_app/shieldml/data

# 设置时区
ENV TZ=Asia/Shanghai

# 从构建阶段复制编译好的程序和HTML文件
COPY --from=builder /build/shieldml_server /www/dk_project/dk_app/shieldml/
COPY --from=builder /build/shieldml_scan.html /www/dk_project/dk_app/shieldml/
COPY bt-shieldml /www/dk_project/dk_app/shieldml/

# 设置权限
RUN chmod +x /www/dk_project/dk_app/shieldml/shieldml_server && \
    chmod +x /www/dk_project/dk_app/shieldml/bt-shieldml && \
    echo '{"results":[]}' > /www/dk_project/dk_app/shieldml/data/webshellJson.json && \
    chmod 755 /www/dk_project/dk_app/shieldml/data/webshellJson.json && \
    chmod 755 /www/dk_project/dk_app/shieldml/data

# 暴露端口
EXPOSE 6528

# 健康检查
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget -qO- http://localhost:6528/shieldml_scan.html || exit 1

# 创建非特权用户
RUN groupadd -r shieldml && useradd -r -g shieldml shieldml
USER shieldml

# 启动服务
CMD ["/www/dk_project/dk_app/shieldml/shieldml_server"]