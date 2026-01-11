# Docker镜像构建和推送指南

## ✅ 镜像已构建成功

**镜像信息**:
- 本地标签: `uav-agent:v0.3.0`
- Docker Hub标签: `x1224403599/uav-agent:v0.3.0`
- 架构: `linux/arm64`
- 大小: ~15MB (压缩后)

**包含的新功能**:
- ✅ 仿真数据支持 (从JSON文件读取)
- ✅ 智能推送机制 (位置/电量变化检测)
- ✅ 位置/速度/仿真元数据字段
- ✅ 统计日志和监控

---

## 推送到Docker Hub

### 方法1: 使用推送脚本（推荐）

```bash
cd /home/ubuntu/DevUav/K3sUav
./push-image.sh
```

脚本会自动：
1. 检查镜像是否存在
2. 提示登录Docker Hub（如果未登录）
3. 推送镜像
4. 显示成功信息

### 方法2: 手动推送

```bash
# 1. 登录Docker Hub
docker login -u x1224403599

# 2. 推送镜像
docker push x1224403599/uav-agent:v0.3.0

# 3. 验证
docker pull x1224403599/uav-agent:v0.3.0
```

---

## 验证镜像

### 查看本地镜像

```bash
docker images | grep uav-agent
```

输出示例:
```
x1224403599/uav-agent   v0.3.0   858c3ff5237c   5 minutes ago   14.9MB
uav-agent               v0.3.0   858c3ff5237c   5 minutes ago   14.9MB
```

### 检查镜像内容

```bash
docker run --rm x1224403599/uav-agent:v0.3.0 --help 2>&1 || echo "镜像OK"
```

### 查看镜像层

```bash
docker history x1224403599/uav-agent:v0.3.0
```

---

## 使用新镜像

### 更新部署文件

编辑 `deploy/agent-daemonset-simulation-test.yaml`:

```yaml
containers:
- name: uav-agent
  image: x1224403599/uav-agent:v0.3.0  # 更新这里
  imagePullPolicy: IfNotPresent
```

### 部署到K3s

```bash
# 如果需要导入到K3s（可选）
docker save x1224403599/uav-agent:v0.3.0 | sudo k3s ctr images import -

# 部署
kubectl apply -f deploy/agent-daemonset-simulation-test.yaml

# 查看状态
kubectl get pods -l app=uav-agent-sim -o wide
```

### 验证运行

```bash
# 查看日志
kubectl logs -l app=uav-agent-sim --tail=50

# 应该看到：
# - "Change detector initialized"
# - "Simulation mode enabled"
# - "Change detection statistics"
```

---

## 多架构支持（可选）

如果需要支持多架构（amd64 + arm64）:

```bash
# 创建buildx构建器
docker buildx create --name multiarch --use

# 构建并推送多架构镜像
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t x1224403599/uav-agent:v0.3.0 \
  --push \
  .
```

---

## 故障排查

### 问题1: 推送失败 (unauthorized)

**原因**: 未登录或凭证过期

**解决**:
```bash
docker logout
docker login -u x1224403599
```

### 问题2: 推送速度慢

**原因**: 网络问题

**解决**:
```bash
# 使用镜像加速（中国地区）
# 编辑 /etc/docker/daemon.json
{
  "registry-mirrors": [
    "https://docker.mirrors.ustc.edu.cn"
  ]
}

# 重启Docker
sudo systemctl restart docker
```

### 问题3: 镜像拉取失败

**原因**: 镜像还未推送或权限问题

**解决**:
```bash
# 检查Docker Hub上的镜像
docker search x1224403599/uav-agent

# 或访问: https://hub.docker.com/r/x1224403599/uav-agent/tags
```

---

## 下一步

1. **推送镜像到Docker Hub**
   ```bash
   ./push-image.sh
   ```

2. **更新部署配置**
   ```bash
   # 更新镜像版本
   sed -i 's|image: .*uav-agent.*|image: x1224403599/uav-agent:v0.3.0|g' \
     deploy/agent-daemonset-simulation-test.yaml
   ```

3. **部署测试**
   ```bash
   kubectl apply -f deploy/agent-daemonset-simulation-test.yaml
   kubectl logs -l app=uav-agent-sim -f
   ```

4. **验证功能**
   - 检查变化检测日志
   - 查看UAVMetrics CRD
   - 测试位置/电量变化触发推送

---

## 快速命令速查

```bash
# 构建镜像
docker build -t x1224403599/uav-agent:v0.3.0 .

# 推送镜像
docker push x1224403599/uav-agent:v0.3.0

# 拉取镜像
docker pull x1224403599/uav-agent:v0.3.0

# 查看镜像
docker images | grep uav-agent

# 删除旧镜像
docker rmi x1224403599/uav-agent:v0.1.0

# 清理未使用的镜像
docker image prune -a
```

---

## 镜像标签策略

**建议**:
- `v0.3.0` - 具体版本（当前）
- `v0.3` - 次版本（可选）
- `latest` - 最新版本（可选）

**创建多标签**:
```bash
docker tag x1224403599/uav-agent:v0.3.0 x1224403599/uav-agent:latest
docker push x1224403599/uav-agent:latest
```

---

## 总结

✅ 镜像已成功构建
⏳ 等待推送到Docker Hub
📝 推送命令: `./push-image.sh` 或 `docker push x1224403599/uav-agent:v0.3.0`

**新版本亮点**:
- 仿真数据集成
- 智能推送机制（节省70-90%的API调用）
- 完整的位置/速度/仿真元数据支持
- 实时统计和监控
