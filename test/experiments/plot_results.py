#!/usr/bin/env python3
"""
实验结果可视化脚本
生成状态预测器和异常检测器的实验图表
"""

import pandas as pd
import matplotlib.pyplot as plt
import matplotlib
import os

# 设置中文字体
matplotlib.rcParams['font.sans-serif'] = ['SimHei', 'DejaVu Sans', 'Arial Unicode MS']
matplotlib.rcParams['axes.unicode_minus'] = False

# 设置图表风格
plt.style.use('seaborn-v0_8-whitegrid')

def plot_prediction_accuracy():
    """绘制预测准确性图表"""
    try:
        df = pd.read_csv('prediction_accuracy.csv')
    except FileNotFoundError:
        print("prediction_accuracy.csv not found, skipping...")
        return

    fig, axes = plt.subplots(2, 2, figsize=(14, 10))

    # 图1: 预测误差 vs 数据年龄
    ax1 = axes[0, 0]
    ax1.plot(df['data_age_seconds'], df['battery_mae'], 'b-o', label='Battery MAE (%)', linewidth=2, markersize=6)
    ax1.set_xlabel('Data Age (seconds)', fontsize=12)
    ax1.set_ylabel('Battery MAE (%)', fontsize=12, color='b')
    ax1.tick_params(axis='y', labelcolor='b')
    ax1.set_title('Battery Prediction Error vs Data Age', fontsize=14, fontweight='bold')
    ax1.grid(True, alpha=0.3)

    ax1_twin = ax1.twinx()
    ax1_twin.plot(df['data_age_seconds'], df['position_mae'], 'r-s', label='Position MAE (m)', linewidth=2, markersize=6)
    ax1_twin.set_ylabel('Position MAE (m)', fontsize=12, color='r')
    ax1_twin.tick_params(axis='y', labelcolor='r')

    # 图2: 置信度衰减
    ax2 = axes[0, 1]
    ax2.plot(df['data_age_seconds'], df['battery_confidence'], 'b-o', label='Battery', linewidth=2, markersize=6)
    ax2.plot(df['data_age_seconds'], df['position_confidence'], 'r-s', label='Position', linewidth=2, markersize=6)
    ax2.set_xlabel('Data Age (seconds)', fontsize=12)
    ax2.set_ylabel('Confidence', fontsize=12)
    ax2.set_title('Prediction Confidence Decay', fontsize=14, fontweight='bold')
    ax2.legend(loc='upper right', fontsize=10)
    ax2.set_ylim(0, 1.1)
    ax2.grid(True, alpha=0.3)

    # 图3: 延迟预测误差
    ax3 = axes[1, 0]
    ax3.bar(df['data_age_seconds'], df['latency_mae'], color='steelblue', alpha=0.7, edgecolor='black')
    ax3.set_xlabel('Data Age (seconds)', fontsize=12)
    ax3.set_ylabel('Latency MAE (ms)', fontsize=12)
    ax3.set_title('Latency Prediction Error', fontsize=14, fontweight='bold')
    ax3.grid(True, alpha=0.3, axis='y')

    # 图4: 综合误差热力图
    ax4 = axes[1, 1]
    # 归一化误差
    battery_norm = df['battery_mae'] / df['battery_mae'].max()
    position_norm = df['position_mae'] / df['position_mae'].max() if df['position_mae'].max() > 0 else df['position_mae']
    latency_norm = df['latency_mae'] / df['latency_mae'].max() if df['latency_mae'].max() > 0 else df['latency_mae']

    data = [battery_norm.values, position_norm.values, latency_norm.values]
    im = ax4.imshow(data, aspect='auto', cmap='RdYlGn_r')
    ax4.set_yticks([0, 1, 2])
    ax4.set_yticklabels(['Battery', 'Position', 'Latency'])
    ax4.set_xticks(range(len(df)))
    ax4.set_xticklabels(df['data_age_seconds'].values)
    ax4.set_xlabel('Data Age (seconds)', fontsize=12)
    ax4.set_title('Normalized Prediction Error Heatmap', fontsize=14, fontweight='bold')
    plt.colorbar(im, ax=ax4, label='Normalized Error')

    plt.tight_layout()
    plt.savefig('prediction_accuracy_plot.png', dpi=150, bbox_inches='tight')
    print("Saved: prediction_accuracy_plot.png")
    plt.close()

def plot_scenario_comparison():
    """绘制场景对比图"""
    try:
        df = pd.read_csv('scenario_comparison.csv')
    except FileNotFoundError:
        print("scenario_comparison.csv not found, skipping...")
        return

    fig, axes = plt.subplots(1, 2, figsize=(12, 5))

    # 图1: 不同场景的电量预测误差
    ax1 = axes[0]
    bars = ax1.bar(df['scenario'], df['battery_mae'], color=['#2ecc71', '#3498db', '#f39c12', '#e74c3c', '#9b59b6'])
    ax1.set_xlabel('Flight Scenario', fontsize=12)
    ax1.set_ylabel('Battery MAE (%)', fontsize=12)
    ax1.set_title('Battery Prediction Error by Scenario', fontsize=14, fontweight='bold')
    ax1.tick_params(axis='x', rotation=45)
    for bar, val in zip(bars, df['battery_mae']):
        ax1.text(bar.get_x() + bar.get_width()/2, bar.get_height() + 0.01, f'{val:.3f}',
                ha='center', va='bottom', fontsize=9)

    # 图2: 不同场景的位置预测误差
    ax2 = axes[1]
    bars = ax2.bar(df['scenario'], df['position_mae'], color=['#2ecc71', '#3498db', '#f39c12', '#e74c3c', '#9b59b6'])
    ax2.set_xlabel('Flight Scenario', fontsize=12)
    ax2.set_ylabel('Position MAE (m)', fontsize=12)
    ax2.set_title('Position Prediction Error by Scenario', fontsize=14, fontweight='bold')
    ax2.tick_params(axis='x', rotation=45)
    for bar, val in zip(bars, df['position_mae']):
        ax2.text(bar.get_x() + bar.get_width()/2, bar.get_height() + 0.5, f'{val:.2f}',
                ha='center', va='bottom', fontsize=9)

    plt.tight_layout()
    plt.savefig('scenario_comparison_plot.png', dpi=150, bbox_inches='tight')
    print("Saved: scenario_comparison_plot.png")
    plt.close()

def plot_confidence_decay():
    """绘制置信度衰减曲线"""
    try:
        df = pd.read_csv('confidence_decay.csv')
    except FileNotFoundError:
        print("confidence_decay.csv not found, skipping...")
        return

    fig, ax = plt.subplots(figsize=(10, 6))

    ax.plot(df['time_seconds'], df['battery_confidence'], 'b-o', label='Battery (half-life=30s)', linewidth=2, markersize=5)
    ax.plot(df['time_seconds'], df['position_confidence'], 'r-s', label='Position (half-life=10s)', linewidth=2, markersize=5)
    ax.plot(df['time_seconds'], df['latency_confidence'], 'g-^', label='Latency (half-life=20s)', linewidth=2, markersize=5)

    # 添加半衰期标记线
    ax.axhline(y=0.5, color='gray', linestyle='--', alpha=0.5, label='Half-life threshold')
    ax.axvline(x=30, color='blue', linestyle=':', alpha=0.3)
    ax.axvline(x=10, color='red', linestyle=':', alpha=0.3)
    ax.axvline(x=20, color='green', linestyle=':', alpha=0.3)

    ax.set_xlabel('Time Since Last Update (seconds)', fontsize=12)
    ax.set_ylabel('Confidence', fontsize=12)
    ax.set_title('Prediction Confidence Decay Over Time', fontsize=14, fontweight='bold')
    ax.legend(loc='upper right', fontsize=10)
    ax.set_ylim(0, 1.05)
    ax.set_xlim(0, 120)
    ax.grid(True, alpha=0.3)

    plt.tight_layout()
    plt.savefig('confidence_decay_plot.png', dpi=150, bbox_inches='tight')
    print("Saved: confidence_decay_plot.png")
    plt.close()

def plot_detection_rate():
    """绘制检测率图表"""
    try:
        df = pd.read_csv('detection_rate.csv')
    except FileNotFoundError:
        print("detection_rate.csv not found, skipping...")
        return

    fig, axes = plt.subplots(1, 2, figsize=(14, 5))

    # 图1: 检测率条形图
    ax1 = axes[0]
    colors = plt.cm.RdYlGn(df['detection_rate'] / 100)
    bars = ax1.barh(df['anomaly_type'], df['detection_rate'], color=colors, edgecolor='black')
    ax1.set_xlabel('Detection Rate (%)', fontsize=12)
    ax1.set_ylabel('Anomaly Type', fontsize=12)
    ax1.set_title('Anomaly Detection Rate by Type', fontsize=14, fontweight='bold')
    ax1.set_xlim(0, 105)
    for bar, val in zip(bars, df['detection_rate']):
        ax1.text(val + 1, bar.get_y() + bar.get_height()/2, f'{val:.1f}%',
                ha='left', va='center', fontsize=10)

    # 图2: 检测分数分布
    ax2 = axes[1]
    bars = ax2.bar(df['anomaly_type'], df['avg_score'], color='steelblue', alpha=0.7, edgecolor='black')
    ax2.set_xlabel('Anomaly Type', fontsize=12)
    ax2.set_ylabel('Average Anomaly Score', fontsize=12)
    ax2.set_title('Average Anomaly Score by Type', fontsize=14, fontweight='bold')
    ax2.tick_params(axis='x', rotation=45)
    ax2.set_ylim(0, 1.1)
    for bar, val in zip(bars, df['avg_score']):
        ax2.text(bar.get_x() + bar.get_width()/2, bar.get_height() + 0.02, f'{val:.3f}',
                ha='center', va='bottom', fontsize=9)

    plt.tight_layout()
    plt.savefig('detection_rate_plot.png', dpi=150, bbox_inches='tight')
    print("Saved: detection_rate_plot.png")
    plt.close()

def plot_false_positive():
    """绘制误报率图表"""
    try:
        df = pd.read_csv('false_positive.csv')
    except FileNotFoundError:
        print("false_positive.csv not found, skipping...")
        return

    fig, ax = plt.subplots(figsize=(10, 5))

    ax.bar(df['test_round'], df['fp_rate'], color='coral', alpha=0.7, edgecolor='black', label='False Positive Rate')
    ax.axhline(y=df['fp_rate'].mean(), color='red', linestyle='--', linewidth=2, label=f'Average: {df["fp_rate"].mean():.2f}%')

    ax.set_xlabel('Test Round', fontsize=12)
    ax.set_ylabel('False Positive Rate (%)', fontsize=12)
    ax.set_title('False Positive Rate Across Test Rounds', fontsize=14, fontweight='bold')
    ax.legend(loc='upper right', fontsize=10)
    ax.set_ylim(0, max(df['fp_rate'].max() * 1.3, 10))

    plt.tight_layout()
    plt.savefig('false_positive_plot.png', dpi=150, bbox_inches='tight')
    print("Saved: false_positive_plot.png")
    plt.close()

def plot_detection_latency():
    """绘制检测延迟图表"""
    try:
        df = pd.read_csv('detection_latency.csv')
    except FileNotFoundError:
        print("detection_latency.csv not found, skipping...")
        return

    fig, ax = plt.subplots(figsize=(10, 6))

    x = range(len(df))
    width = 0.25

    ax.bar([i - width for i in x], df['min_latency_us'], width, label='Min', color='#2ecc71', alpha=0.8)
    ax.bar(x, df['avg_latency_us'], width, label='Average', color='#3498db', alpha=0.8)
    ax.bar([i + width for i in x], df['max_latency_us'], width, label='Max', color='#e74c3c', alpha=0.8)

    ax.set_xlabel('Detector Type', fontsize=12)
    ax.set_ylabel('Latency (μs)', fontsize=12)
    ax.set_title('Detection Latency by Detector Type', fontsize=14, fontweight='bold')
    ax.set_xticks(x)
    ax.set_xticklabels(df['detector_type'])
    ax.legend(loc='upper right', fontsize=10)
    ax.grid(True, alpha=0.3, axis='y')

    plt.tight_layout()
    plt.savefig('detection_latency_plot.png', dpi=150, bbox_inches='tight')
    print("Saved: detection_latency_plot.png")
    plt.close()

def plot_if_training():
    """绘制Isolation Forest训练效果图"""
    try:
        df = pd.read_csv('if_training.csv')
    except FileNotFoundError:
        print("if_training.csv not found, skipping...")
        return

    fig, ax = plt.subplots(figsize=(10, 6))

    ax.plot(df['training_samples'], df['detection_rate'], 'b-o', label='Detection Rate', linewidth=2, markersize=8)
    ax.plot(df['training_samples'], df['false_positive_rate'], 'r-s', label='False Positive Rate', linewidth=2, markersize=8)

    ax.fill_between(df['training_samples'], df['detection_rate'], alpha=0.3, color='blue')
    ax.fill_between(df['training_samples'], df['false_positive_rate'], alpha=0.3, color='red')

    ax.set_xlabel('Number of Training Samples', fontsize=12)
    ax.set_ylabel('Rate (%)', fontsize=12)
    ax.set_title('Isolation Forest: Detection Rate vs Training Size', fontsize=14, fontweight='bold')
    ax.legend(loc='center right', fontsize=10)
    ax.set_ylim(0, 105)
    ax.grid(True, alpha=0.3)

    plt.tight_layout()
    plt.savefig('if_training_plot.png', dpi=150, bbox_inches='tight')
    print("Saved: if_training_plot.png")
    plt.close()

def create_summary_figure():
    """创建综合摘要图"""
    fig = plt.figure(figsize=(16, 12))

    # 尝试读取所有数据
    try:
        pred_df = pd.read_csv('prediction_accuracy.csv')
        det_df = pd.read_csv('detection_rate.csv')
        conf_df = pd.read_csv('confidence_decay.csv')
        fp_df = pd.read_csv('false_positive.csv')
    except FileNotFoundError as e:
        print(f"Missing data file: {e}")
        return

    # 子图1: 预测误差
    ax1 = fig.add_subplot(2, 2, 1)
    ax1.plot(pred_df['data_age_seconds'], pred_df['battery_mae'], 'b-o', label='Battery', linewidth=2)
    ax1.plot(pred_df['data_age_seconds'], pred_df['position_mae']/10, 'r-s', label='Position/10', linewidth=2)
    ax1.set_xlabel('Data Age (s)')
    ax1.set_ylabel('MAE')
    ax1.set_title('(a) Prediction Error vs Data Age')
    ax1.legend()
    ax1.grid(True, alpha=0.3)

    # 子图2: 置信度衰减
    ax2 = fig.add_subplot(2, 2, 2)
    ax2.plot(conf_df['time_seconds'], conf_df['battery_confidence'], 'b-', label='Battery', linewidth=2)
    ax2.plot(conf_df['time_seconds'], conf_df['position_confidence'], 'r-', label='Position', linewidth=2)
    ax2.plot(conf_df['time_seconds'], conf_df['latency_confidence'], 'g-', label='Latency', linewidth=2)
    ax2.axhline(y=0.5, color='gray', linestyle='--', alpha=0.5)
    ax2.set_xlabel('Time (s)')
    ax2.set_ylabel('Confidence')
    ax2.set_title('(b) Confidence Decay')
    ax2.legend()
    ax2.grid(True, alpha=0.3)

    # 子图3: 检测率
    ax3 = fig.add_subplot(2, 2, 3)
    colors = plt.cm.RdYlGn(det_df['detection_rate'] / 100)
    ax3.barh(det_df['anomaly_type'], det_df['detection_rate'], color=colors)
    ax3.set_xlabel('Detection Rate (%)')
    ax3.set_title('(c) Anomaly Detection Rate')
    ax3.set_xlim(0, 105)

    # 子图4: 误报率
    ax4 = fig.add_subplot(2, 2, 4)
    ax4.bar(fp_df['test_round'], fp_df['fp_rate'], color='coral', alpha=0.7)
    ax4.axhline(y=fp_df['fp_rate'].mean(), color='red', linestyle='--', linewidth=2)
    ax4.set_xlabel('Test Round')
    ax4.set_ylabel('False Positive Rate (%)')
    ax4.set_title(f'(d) False Positive Rate (Avg: {fp_df["fp_rate"].mean():.2f}%)')

    plt.suptitle('AI Modules Experimental Validation Summary', fontsize=16, fontweight='bold', y=1.02)
    plt.tight_layout()
    plt.savefig('experiment_summary.png', dpi=150, bbox_inches='tight')
    print("Saved: experiment_summary.png")
    plt.close()

if __name__ == '__main__':
    print("Generating experiment plots...")
    print("=" * 50)

    # 切换到实验数据目录
    script_dir = os.path.dirname(os.path.abspath(__file__))
    os.chdir(script_dir)

    plot_prediction_accuracy()
    plot_scenario_comparison()
    plot_confidence_decay()
    plot_detection_rate()
    plot_false_positive()
    plot_detection_latency()
    plot_if_training()
    create_summary_figure()

    print("=" * 50)
    print("All plots generated!")
