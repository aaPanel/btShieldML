# 使用Debian作为基础镜像
FROM debian:11-slim

# 设置工作目录
WORKDIR /www/dk_project/dk_app/shieldml/

# 安装必要的依赖
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