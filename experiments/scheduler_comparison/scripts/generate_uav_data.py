#!/usr/bin/env python3
"""
生成 50 台无人机的 UAVMetrics 模拟数据
- GPS: 蜂窝状圆形编队，相邻间距 200m
- 电量: 随机 30%-90%
- 网络延迟: 随机 10-150ms

复用自 UavRouting 项目的数据生成脚本
"""

import math
import random
import csv
import os

try:
    import yaml
    HAS_YAML = True
except ImportError:
    HAS_YAML = False
    print("⚠️  yaml 模块未安装，将跳过 YAML 文件生成")

# 配置参数
NODE_COUNT = 50  # 50个无人机节点
SPACING = 200  # 相邻间距 200米
BASE_LAT = 39.9042  # 圆心纬度 (北京)
BASE_LON = 116.4074  # 圆心经度

# 米转经纬度的近似系数 (北京附近)
METERS_PER_LAT_DEGREE = 111000
METERS_PER_LON_DEGREE = 111000 * math.cos(math.radians(BASE_LAT))


def generate_hexagonal_grid(n_nodes, spacing):
    """
    生成蜂窝状六边形网格坐标
    返回 (x, y) 列表，单位：米
    """
    points = [(0, 0)]  # 中心点

    # 六边形方向 (60度间隔)
    directions = [
        (1, 0),
        (0.5, math.sqrt(3)/2),
        (-0.5, math.sqrt(3)/2),
        (-1, 0),
        (-0.5, -math.sqrt(3)/2),
        (0.5, -math.sqrt(3)/2),
    ]

    ring = 1
    while len(points) < n_nodes:
        x, y = ring * spacing, 0

        for dir_idx in range(6):
            dx, dy = directions[(dir_idx + 2) % 6]
            for _ in range(ring):
                if len(points) >= n_nodes:
                    break
                points.append((x, y))
                x += dx * spacing
                y += dy * spacing

        ring += 1

    return points[:n_nodes]


def meters_to_gps(x_meters, y_meters, base_lat, base_lon):
    """将相对米坐标转换为 GPS 经纬度"""
    lat = base_lat + (y_meters / METERS_PER_LAT_DEGREE)
    lon = base_lon + (x_meters / METERS_PER_LON_DEGREE)
    return round(lat, 6), round(lon, 6)


def main():
    random.seed(42)  # 固定随机种子，便于复现

    grid_points = generate_hexagonal_grid(NODE_COUNT, SPACING)

    print(f"生成 {NODE_COUNT} 个节点的 UAVMetrics 数据")
    print(f"编队参数: 蜂窝状圆形, 间距 {SPACING}m")
    print(f"圆心坐标: ({BASE_LAT}, {BASE_LON})")
    print(f"电量范围: 15% - 95%（方差更大）")
    print(f"延迟规律: 随距离增加而增加（距圆心越远，延迟越高）")
    print("-" * 70)

    # 确保目录存在
    os.makedirs("data", exist_ok=True)

    # 生成节点数据
    nodes_data = []

    for i, (x, y) in enumerate(grid_points):
        lat, lon = meters_to_gps(x, y, BASE_LAT, BASE_LON)

        node_name = f"uav-node-{i+1:02d}"
        distance_from_center = math.sqrt(x**2 + y**2)

        # 电量：方差更大，范围 15%-95%
        battery = random.randint(15, 95)

        # 延迟：与距离相关（距圆心越远，延迟越高）
        # 公式：基础延迟 + 距离因子 * 距离 + 随机扰动
        base_latency = 10.0  # 基础延迟 10ms
        distance_factor = 0.08  # 每米增加 0.08ms
        distance_latency = distance_factor * distance_from_center
        noise = random.uniform(-15, 25)  # 随机扰动 -15 到 +25ms
        latency = max(5.0, base_latency + distance_latency + noise)  # 最低5ms
        latency = round(latency, 2)

        cpu_usage = round(random.uniform(5, 60), 2)      # CPU使用率 5%-60%
        memory_usage = round(random.uniform(20, 80), 2)   # 内存使用率 20%-80%

        nodes_data.append({
            'node_name': node_name,
            'latitude': lat,
            'longitude': lon,
            'x_meters': x,
            'y_meters': y,
            'battery_percent': battery,
            'network_latency_ms': latency,
            'cpu_usage_percent': cpu_usage,
            'memory_usage_percent': memory_usage,
        })

        print(f"{node_name:15} | "
              f"GPS: ({lat:8.4f}, {lon:9.4f}) | "
              f"距圆心: {distance_from_center:6.1f}m | "
              f"电量: {battery:3d}% | "
              f"延迟: {latency:6.2f}ms | "
              f"CPU: {cpu_usage:5.1f}% | "
              f"内存: {memory_usage:5.1f}%")

    # 写入 CSV 文件
    csv_file = "data/uavmetrics.csv"
    with open(csv_file, "w", newline='') as f:
        writer = csv.DictWriter(f, fieldnames=[
            'node_name', 'latitude', 'longitude',
            'x_meters', 'y_meters', 'battery_percent', 'network_latency_ms',
            'cpu_usage_percent', 'memory_usage_percent'
        ])
        writer.writeheader()
        writer.writerows(nodes_data)

    print("-" * 70)
    print(f"✓ CSV 已生成: {csv_file}")

    # 可选: 生成 Kubernetes CRD YAML
    if HAS_YAML:
        yaml_file = "data/uavmetrics.yaml"
        crd_objects = []

        for node_data in nodes_data:
            crd = {
                "apiVersion": "uav.k3s.io/v1alpha1",
                "kind": "UAVMetrics",
                "metadata": {
                    "name": node_data['node_name'],
                    "namespace": "default",
                },
                "spec": {
                    "nodeName": node_data['node_name'],
                    "gps": {
                        "latitude": node_data['latitude'],
                        "longitude": node_data['longitude'],
                        "altitude": round(random.uniform(50, 150), 2),
                        "accuracy": round(random.uniform(1, 5), 2),
                        "satellites": random.randint(8, 14),
                    },
                    "battery": {
                        "remainingPercent": float(node_data['battery_percent']),
                        "voltage": round(11.1 + (node_data['battery_percent'] / 100) * 1.5, 2),
                        "temperature": round(random.uniform(25, 35), 2),
                    },
                    "network": {
                        "latency": node_data['network_latency_ms'],
                        "signalStrength": random.randint(-80, -40),
                        "bandwidth": round(random.uniform(20, 100), 2),
                    },
                    "performance": {
                        "cpuUsage": round(random.uniform(5, 30), 2),
                        "memoryUsage": round(random.uniform(10, 40), 2),
                    },
                    "health": {
                        "status": "Healthy",
                    }
                },
                "status": {
                    "phase": "Active",
                }
            }
            crd_objects.append(crd)

        with open(yaml_file, "w") as f:
            yaml.dump_all(crd_objects, f, default_flow_style=False, allow_unicode=True)

        print(f"✓ YAML 已生成: {yaml_file}")

    # 统计信息
    print("-" * 70)
    print("统计信息:")
    print(f"  节点总数: {len(nodes_data)}")
    print(f"  平均电量: {sum(n['battery_percent'] for n in nodes_data) / len(nodes_data):.1f}%")
    print(f"  电量范围: {min(n['battery_percent'] for n in nodes_data)}% - {max(n['battery_percent'] for n in nodes_data)}%")
    print(f"  平均延迟: {sum(n['network_latency_ms'] for n in nodes_data) / len(nodes_data):.2f}ms")
    print(f"  编队半径: {max(math.sqrt(n['x_meters']**2 + n['y_meters']**2) for n in nodes_data):.1f}m")


if __name__ == "__main__":
    main()
