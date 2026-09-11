#!/bin/bash
# Happy-Cloud NAS 部署脚本（在 NAS 上以 sudo 执行）
set -x

DEPLOY_DIR=/opt/happy-cloud
STAGING=/volume1/download/happy-cloud-deploy

echo "==== [1/6] 检查暂存文件 ===="
ls -lh "$STAGING"

echo "==== [2/6] 移动文件到部署目录 ===="
cp "$STAGING/docker-compose.yml" "$DEPLOY_DIR/docker-compose.yml"
cp "$STAGING/happy-cloud-arm64.tar" "$DEPLOY_DIR/happy-cloud-arm64.tar"

echo "==== [3/6] 查看旧容器挂载卷 ===="
docker inspect happy-cloud-backend happy-cloud-web happy-cloud-mysql happy-cloud-redis --format '{{.Name}} mounts:' 2>/dev/null
for c in happy-cloud-backend happy-cloud-web happy-cloud-mysql happy-cloud-redis; do
  echo "--- $c ---"
  docker inspect "$c" --format '{{range .Mounts}}name={{.Name}} type={{.Type}} src={{.Source}} dst={{.Destination}}{{println}}{{end}}' 2>/dev/null || echo "no inspect"
done
docker volume ls | grep -i happy || echo "no happy volumes"

echo "==== [4/6] 停止并移除旧容器 ===="
docker stop happy-cloud-backend happy-cloud-web happy-cloud-mysql happy-cloud-redis 2>/dev/null || echo "stop done/failed"
docker rm happy-cloud-backend happy-cloud-web happy-cloud-mysql happy-cloud-redis 2>/dev/null || echo "rm done/failed"

echo "==== [5/6] 加载新 arm64 镜像 ===="
docker load -i "$DEPLOY_DIR/happy-cloud-arm64.tar"
docker images | grep happy-cloud

echo "==== [6/6] 启动 compose 服务 ===="
cd "$DEPLOY_DIR" || exit 1
docker compose up -d

echo "==== 部署命令执行完毕 ===="
