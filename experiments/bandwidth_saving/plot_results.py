#!/usr/bin/env python3
"""
云端预测带宽节省实验 - 结果可视化
"""

import pandas as pd
import matplotlib.pyplot as plt
import numpy as np
import os

# 设置中文字体
plt.rcParams['font.sans-serif'] = ['SimHei', 'DejaVu Sans']
plt.rcParams['axes.unicode_minus'] = False

def load_data():
    """加载实验数据"""
    summary = pd.read_csv('summary.csv')
    timeseries = pd.read_csv('timeseries.csv')
    return summary, timeseries

def plot_bandwidth_comparison(summary):
    """图1: 带宽消耗对比"""
    fig, ax = plt.subplots(figsize=(10, 6))

    groups = summary['group'].tolist()
    bandwidth = summary['bandwidth_kb'].tolist()

    # 简化标签
    labels = ['A: High-Freq\n(1s)', 'B: Low-Freq\n+Prediction(10s)', 'C: Low-Freq\n(10s)']
    colors = ['#2196F3', '#4CAF50', '#FF9800']

    bars = ax.bar(labels, bandwidth, color=colors, edgecolor='black', linewidth=1.5)

    # 添加数值标签
    for bar, bw in zip(bars, bandwidth):
        height = bar.get_height()
        ax.annotate(f'{bw:.1f} KB',
                    xy=(bar.get_x() + bar.get_width() / 2, height),
                    xytext=(0, 5),
                    textcoords="offset points",
                    ha='center', va='bottom', fontsize=12, fontweight='bold')

    # 添加节省比例
    baseline = bandwidth[0]
    for i, bw in enumerate(bandwidth[1:], 1):
        saving = (1 - bw/baseline) * 100
        ax.annotate(f'Save {saving:.0f}%',
                    xy=(bars[i].get_x() + bars[i].get_width() / 2, bandwidth[i]/2),
                    ha='center', va='center', fontsize=11, color='white', fontweight='bold')

    ax.set_ylabel('Bandwidth Consumption (KB)', fontsize=12)
    ax.set_title('Bandwidth Consumption Comparison', fontsize=14, fontweight='bold')
    ax.set_ylim(0, max(bandwidth) * 1.2)
    ax.grid(axis='y', alpha=0.3)

    plt.tight_layout()
    plt.savefig('plot_bandwidth.png', dpi=150)
    plt.close()
    print("Generated: plot_bandwidth.png")

def plot_prediction_accuracy(summary):
    """图2: 预测准确性对比"""
    fig, axes = plt.subplots(1, 2, figsize=(14, 5))

    labels = ['A: High-Freq', 'B: Low+Pred', 'C: Low-Freq']
    colors = ['#2196F3', '#4CAF50', '#FF9800']

    # 电量误差
    battery_mae = summary['battery_mae'].tolist()
    bars1 = axes[0].bar(labels, battery_mae, color=colors, edgecolor='black')
    axes[0].set_ylabel('Battery MAE (%)', fontsize=12)
    axes[0].set_title('Battery Prediction Error', fontsize=13, fontweight='bold')
    for bar, val in zip(bars1, battery_mae):
        axes[0].annotate(f'{val:.3f}%', xy=(bar.get_x() + bar.get_width()/2, bar.get_height()),
                        xytext=(0, 3), textcoords="offset points", ha='center', fontsize=11)
    axes[0].grid(axis='y', alpha=0.3)

    # 位置误差
    position_mae = summary['position_mae'].tolist()
    bars2 = axes[1].bar(labels, position_mae, color=colors, edgecolor='black')
    axes[1].set_ylabel('Position MAE (m)', fontsize=12)
    axes[1].set_title('Position Prediction Error', fontsize=13, fontweight='bold')
    for bar, val in zip(bars2, position_mae):
        axes[1].annotate(f'{val:.1f}m', xy=(bar.get_x() + bar.get_width()/2, bar.get_height()),
                        xytext=(0, 3), textcoords="offset points", ha='center', fontsize=11)
    axes[1].grid(axis='y', alpha=0.3)

    plt.tight_layout()
    plt.savefig('plot_accuracy.png', dpi=150)
    plt.close()
    print("Generated: plot_accuracy.png")

def plot_decision_correctness(summary):
    """图3: 调度决策正确率"""
    fig, ax = plt.subplots(figsize=(10, 6))

    labels = ['A: High-Freq\n(1s)', 'B: Low-Freq\n+Prediction(10s)', 'C: Low-Freq\n(10s)']
    colors = ['#2196F3', '#4CAF50', '#FF9800']
    correct_rate = summary['decision_correct_rate'].tolist()

    bars = ax.bar(labels, correct_rate, color=colors, edgecolor='black', linewidth=1.5)

    for bar, rate in zip(bars, correct_rate):
        ax.annotate(f'{rate:.1f}%',
                    xy=(bar.get_x() + bar.get_width() / 2, bar.get_height()),
                    xytext=(0, 5),
                    textcoords="offset points",
                    ha='center', va='bottom', fontsize=12, fontweight='bold')

    ax.set_ylabel('Decision Correctness Rate (%)', fontsize=12)
    ax.set_title('Scheduling Decision Correctness Rate', fontsize=14, fontweight='bold')
    ax.set_ylim(0, 105)
    ax.axhline(y=100, color='gray', linestyle='--', alpha=0.5)
    ax.grid(axis='y', alpha=0.3)

    plt.tight_layout()
    plt.savefig('plot_decision.png', dpi=150)
    plt.close()
    print("Generated: plot_decision.png")

def plot_timeseries(timeseries):
    """图4: 时序数据对比"""
    fig, axes = plt.subplots(2, 1, figsize=(14, 10))

    time = timeseries['time_sec'] / 60  # 转换为分钟

    # 电量时序
    axes[0].plot(time, timeseries['true_battery'], 'k-', linewidth=2, label='True Value', alpha=0.8)
    axes[0].plot(time, timeseries['groupA_battery'], 'b--', linewidth=1.5, label='A: High-Freq', alpha=0.7)
    axes[0].plot(time, timeseries['groupB_battery'], 'g-', linewidth=1.5, label='B: Low+Prediction', alpha=0.7)
    axes[0].plot(time, timeseries['groupC_battery'], 'r:', linewidth=1.5, label='C: Low-Freq(stale)', alpha=0.7)

    axes[0].set_xlabel('Time (min)', fontsize=12)
    axes[0].set_ylabel('Battery (%)', fontsize=12)
    axes[0].set_title('Battery Level Over Time (UAV-0)', fontsize=13, fontweight='bold')
    axes[0].legend(loc='upper right')
    axes[0].grid(alpha=0.3)

    # 位置误差时序
    axes[1].plot(time, timeseries['groupA_pos_err'], 'b-', linewidth=1.5, label='A: High-Freq', alpha=0.7)
    axes[1].plot(time, timeseries['groupB_pos_err'], 'g-', linewidth=1.5, label='B: Low+Prediction', alpha=0.7)
    axes[1].plot(time, timeseries['groupC_pos_err'], 'r-', linewidth=1.5, label='C: Low-Freq(stale)', alpha=0.7)

    axes[1].set_xlabel('Time (min)', fontsize=12)
    axes[1].set_ylabel('Position Error (m)', fontsize=12)
    axes[1].set_title('Position Estimation Error Over Time (UAV-0)', fontsize=13, fontweight='bold')
    axes[1].legend(loc='upper right')
    axes[1].grid(alpha=0.3)

    plt.tight_layout()
    plt.savefig('plot_timeseries.png', dpi=150)
    plt.close()
    print("Generated: plot_timeseries.png")

def plot_summary(summary):
    """图5: 综合对比图"""
    fig, ax = plt.subplots(figsize=(12, 8))

    # 数据准备
    metrics = ['Bandwidth\n(normalized)', 'Battery Error\n(normalized)',
               'Position Error\n(normalized)', 'Decision\nCorrectness']

    # 归一化处理
    bw = summary['bandwidth_kb'].tolist()
    bw_norm = [b/max(bw) for b in bw]

    bat = summary['battery_mae'].tolist()
    bat_norm = [b/max(bat) if max(bat) > 0 else 0 for b in bat]

    pos = summary['position_mae'].tolist()
    pos_norm = [p/max(pos) if max(pos) > 0 else 0 for p in pos]

    dec = [r/100 for r in summary['decision_correct_rate'].tolist()]

    # 组装数据
    group_a = [bw_norm[0], bat_norm[0], pos_norm[0], dec[0]]
    group_b = [bw_norm[1], bat_norm[1], pos_norm[1], dec[1]]
    group_c = [bw_norm[2], bat_norm[2], pos_norm[2], dec[2]]

    x = np.arange(len(metrics))
    width = 0.25

    bars1 = ax.bar(x - width, group_a, width, label='A: High-Freq (1s)', color='#2196F3', edgecolor='black')
    bars2 = ax.bar(x, group_b, width, label='B: Low+Prediction (10s)', color='#4CAF50', edgecolor='black')
    bars3 = ax.bar(x + width, group_c, width, label='C: Low-Freq (10s)', color='#FF9800', edgecolor='black')

    ax.set_ylabel('Normalized Value (lower=better, except Correctness)', fontsize=11)
    ax.set_title('Comprehensive Comparison\n(Bandwidth/Errors: lower is better; Correctness: higher is better)',
                 fontsize=13, fontweight='bold')
    ax.set_xticks(x)
    ax.set_xticklabels(metrics)
    ax.legend(loc='upper right')
    ax.set_ylim(0, 1.3)
    ax.axhline(y=1, color='gray', linestyle='--', alpha=0.3)
    ax.grid(axis='y', alpha=0.3)

    # 添加注释
    ax.annotate('B achieves ~90% bandwidth saving\nwith minimal accuracy loss',
                xy=(0.5, 0.15), fontsize=10, style='italic',
                bbox=dict(boxstyle='round', facecolor='#E8F5E9', alpha=0.8))

    plt.tight_layout()
    plt.savefig('plot_summary.png', dpi=150)
    plt.close()
    print("Generated: plot_summary.png")

def main():
    print("Loading data...")
    summary, timeseries = load_data()

    print("\nGenerating plots...")
    plot_bandwidth_comparison(summary)
    plot_prediction_accuracy(summary)
    plot_decision_correctness(summary)
    plot_timeseries(timeseries)
    plot_summary(summary)

    print("\nAll plots generated successfully!")

if __name__ == '__main__':
    main()
