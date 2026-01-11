#!/bin/bash

# UAV Agent v0.3.0 镜像推送脚本

echo "======================================"
echo " UAV Agent v0.3.0 镜像推送"
echo "======================================"
echo ""

# 检查镜像是否存在
if ! docker images | grep -q "x1224403599/uav-agent.*v0.3.0"; then
    echo "❌ 错误：未找到镜像 x1224403599/uav-agent:v0.3.0"
    echo "请先运行: docker build -t x1224403599/uav-agent:v0.3.0 ."
    exit 1
fi

echo "✅ 找到镜像: x1224403599/uav-agent:v0.3.0"
echo ""

# 检查是否已登录
if ! grep -q "x1224403599" ~/.docker/config.json 2>/dev/null; then
    echo "⚠️  未登录Docker Hub，需要先登录"
    echo ""
    echo "请执行以下命令登录："
    echo "  docker login -u x1224403599"
    echo ""
    read -p "是否现在登录？(y/n) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        docker login -u x1224403599
        if [ $? -ne 0 ]; then
            echo "❌ 登录失败"
            exit 1
        fi
    else
        echo "❌ 取消推送"
        exit 1
    fi
fi

echo ""
echo "开始推送镜像到 Docker Hub..."
echo ""

# 推送镜像
docker push x1224403599/uav-agent:v0.3.0

if [ $? -eq 0 ]; then
    echo ""
    echo "======================================"
    echo " ✅ 镜像推送成功！"
    echo "======================================"
    echo ""
    echo "镜像信息:"
    echo "  仓库: x1224403599/uav-agent"
    echo "  标签: v0.3.0"
    echo "  架构: linux/arm64"
    echo ""
    echo "使用方法:"
    echo "  docker pull x1224403599/uav-agent:v0.3.0"
    echo ""
    echo "Kubernetes部署:"
    echo "  image: x1224403599/uav-agent:v0.3.0"
    echo ""
else
    echo ""
    echo "======================================"
    echo " ❌ 镜像推送失败"
    echo "======================================"
    exit 1
fi
