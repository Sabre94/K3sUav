package main

import (
	"fmt"
	"strings"

	"github.com/k3suav/uav-monitor/pkg/scheduler"
)

func main() {
	fmt.Println("=" + strings.Repeat("=", 79))
	fmt.Println("覆盖率监控过滤测试")
	fmt.Println("=" + strings.Repeat("=", 79))
	fmt.Println()

	// 测试所有算法
	algorithms := []string{
		"distance-based",
		"battery-aware",
		"network-latency",
		"least-loaded",
		"composite",
		"coverage-based",
		"greed-nsgaii",
		"unknown-algorithm",
	}

	fmt.Println("测试 NeedsCoverageMonitor() 函数:")
	fmt.Println(strings.Repeat("-", 60))
	fmt.Printf("%-25s | %s\n", "算法名称", "需要覆盖率监控?")
	fmt.Println(strings.Repeat("-", 60))

	passCount := 0
	failCount := 0

	for _, algo := range algorithms {
		needsMonitor := scheduler.NeedsCoverageMonitor(algo)

		// 期望值：只有 coverage-based 和 greed-nsgaii 需要监控
		expected := (algo == "coverage-based" || algo == "greed-nsgaii")

		status := "✓"
		if needsMonitor != expected {
			status = "✗ (错误!)"
			failCount++
		} else {
			passCount++
		}

		monitorStr := "否"
		if needsMonitor {
			monitorStr = "是"
		}

		fmt.Printf("%-25s | %-8s %s\n", algo, monitorStr, status)
	}

	fmt.Println(strings.Repeat("-", 60))
	fmt.Println()

	// 结果汇总
	fmt.Println("测试结果:")
	fmt.Printf("  通过: %d\n", passCount)
	fmt.Printf("  失败: %d\n", failCount)
	fmt.Println()

	if failCount == 0 {
		fmt.Println("✓ 所有测试通过!")
		fmt.Println()
		fmt.Println("结论:")
		fmt.Println("  - distance-based, battery-aware, network-latency, least-loaded, composite")
		fmt.Println("    这些算法不会触发覆盖率监控，正常调度即可")
		fmt.Println()
		fmt.Println("  - coverage-based, greed-nsgaii")
		fmt.Println("    这两个算法会触发覆盖率监控，支持自适应调度")
	} else {
		fmt.Println("✗ 有测试失败，请检查代码!")
	}
}
