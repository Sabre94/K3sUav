#!/bin/bash

# RISC-V 镜像构建和推送脚本

set -e

DOCKER_USERNAME="x1224403599"
AGENT_IMAGE="${DOCKER_USERNAME}/uav-agent"
SCHEDULER_IMAGE="${DOCKER_USERNAME}/uav-scheduler"
VERSION="v0.1.0-riscv64"

echo "🚀 开始构建 RISC-V 镜像..."

# 检查二进制文件是否存在
if [ ! -f "bin/uav-agent-riscv64" ]; then
    echo "❌ 找不到 bin/uav-agent-riscv64，请先编译"
    exit 1
fi

if [ ! -f "bin/uav-scheduler-riscv64" ]; then
    echo "❌ 找不到 bin/uav-scheduler-riscv64，请先编译"
    exit 1
fi

# 设置 buildx
echo "📦 设置 Docker Buildx..."
docker buildx create --use --name riscv-builder --platform linux/riscv64 2>/dev/null || docker buildx use riscv-builder

# 构建并推送 Agent 镜像
echo "🔨 构建 UAV Agent RISC-V 镜像..."
docker buildx build \
    --platform linux/riscv64 \
    -f Dockerfile.agent-riscv64-simple \
    -t ${AGENT_IMAGE}:${VERSION} \
    -t ${AGENT_IMAGE}:latest-riscv64 \
    --push \
    .

echo "✅ Agent 镜像已推送: ${AGENT_IMAGE}:${VERSION}"

# 构建并推送 Scheduler 镜像
echo "🔨 构建 UAV Scheduler RISC-V 镜像..."
docker buildx build \
    --platform linux/riscv64 \
    -f Dockerfile.scheduler-riscv64-simple \
    -t ${SCHEDULER_IMAGE}:${VERSION} \
    -t ${SCHEDULER_IMAGE}:latest-riscv64 \
    --push \
    .

echo "✅ Scheduler 镜像已推送: ${SCHEDULER_IMAGE}:${VERSION}"

echo ""
echo "🎉 所有镜像构建完成！"
echo ""
echo "推送的镜像："
echo "  - ${AGENT_IMAGE}:${VERSION}"
echo "  - ${AGENT_IMAGE}:latest-riscv64"
echo "  - ${SCHEDULER_IMAGE}:${VERSION}"
echo "  - ${SCHEDULER_IMAGE}:latest-riscv64"
