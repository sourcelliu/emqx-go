package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	projectRoot string
	version     = "4.0.0"
)

func init() {
	// Get project root
	executable, _ := os.Executable()
	projectRoot = filepath.Dir(filepath.Dir(executable))
}

var rootCmd = &cobra.Command{
	Use:   "chaos-cli",
	Short: "EMQX-Go Chaos Engineering Command Line Tool",
	Long: `
┌─────────────────────────────────────────────────────────┐
│  🔥 EMQX-Go Chaos Engineering CLI                       │
│                                                          │
│  Unified command-line interface for chaos testing       │
│  Version: ` + version + `                                         │
└─────────────────────────────────────────────────────────┘

A comprehensive chaos engineering toolkit for EMQX-Go with:
  • 10 chaos test scenarios
  • Real-time monitoring dashboard
  • Automated Game Day runner
  • Performance regression detection
  • Beautiful HTML reports`,
}

// Test command - run chaos tests
var testCmd = &cobra.Command{
	Use:   "test [scenario]",
	Short: "Run chaos test scenarios",
	Long: `Run chaos test scenarios with fault injection.

Available scenarios:
  baseline              - No fault injection (performance baseline)
  network-delay         - 100ms network delay
  high-network-delay    - 500ms network delay
  network-loss          - 10% packet loss
  high-network-loss     - 30% packet loss
  combined-network      - 100ms delay + 10% loss
  cpu-stress            - 80% CPU stress
  extreme-cpu-stress    - 95% CPU stress
  clock-skew            - +5 second clock offset
  cascade-failure       - Multiple simultaneous failures
  all                   - Run all scenarios`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		duration, _ := cmd.Flags().GetInt("duration")
		scenario := "baseline"
		if len(args) > 0 {
			scenario = args[0]
		}

		if scenario == "all" {
			runAllScenarios(duration)
		} else {
			runScenario(scenario, duration)
		}
	},
}

// Dashboard command - start monitoring dashboard
var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Start real-time monitoring dashboard",
	Long:  `Start the web-based real-time monitoring dashboard.`,
	Run: func(cmd *cobra.Command, args []string) {
		port, _ := cmd.Flags().GetInt("port")
		startDashboard(port)
	},
}

// GameDay command - run Game Day exercise
var gamedayCmd = &cobra.Command{
	Use:   "gameday",
	Short: "Run automated Game Day exercise",
	Long: `Run a complete Game Day exercise with 5 progressive phases:
  1. Warm-up (60s) - Baseline testing
  2. Network Resilience (90s) - Network chaos
  3. Resource Pressure (120s) - CPU/Memory stress
  4. Combined Failures (120s) - Mixed faults
  5. Extreme Conditions (180s) - Cascade failures`,
	Run: func(cmd *cobra.Command, args []string) {
		runGameDay()
	},
}

// Report command - generate reports
var reportCmd = &cobra.Command{
	Use:   "report [results-dir]",
	Short: "Generate HTML visualization report",
	Long:  `Generate a beautiful HTML report with interactive charts from test results.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		generateReport(args[0])
	},
}

// Baseline command - manage performance baselines
var baselineCmd = &cobra.Command{
	Use:   "baseline",
	Short: "Manage performance baselines",
	Long:  `Create and compare performance baselines for regression detection.`,
}

var baselineCreateCmd = &cobra.Command{
	Use:   "create [results-dir] [output-file]",
	Short: "Create a new baseline",
	Args:  cobra.RangeArgs(1, 2),
	Run: func(cmd *cobra.Command, args []string) {
		resultsDir := args[0]
		outputFile := "baseline.json"
		if len(args) > 1 {
			outputFile = args[1]
		}
		createBaseline(resultsDir, outputFile)
	},
}

var baselineCompareCmd = &cobra.Command{
	Use:   "compare [baseline-file] [results-dir]",
	Short: "Compare results against baseline",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		compareBaseline(args[0], args[1])
	},
}

// List command - list test results
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List recent test results",
	Long:  `List all recent chaos test results and Game Day exercises.`,
	Run: func(cmd *cobra.Command, args []string) {
		listResults()
	},
}

// Status command - show system status
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show chaos testing system status",
	Long:  `Display current system status, installed tools, and configuration.`,
	Run: func(cmd *cobra.Command, args []string) {
		showStatus()
	},
}

// Clean command - clean up old results
var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Clean up old test results",
	Long:  `Remove old test results and temporary files.`,
	Run: func(cmd *cobra.Command, args []string) {
		days, _ := cmd.Flags().GetInt("days")
		cleanResults(days)
	},
}

func runScenario(scenario string, duration int) {
	fmt.Printf("🔥 Running chaos scenario: %s\n", scenario)
	fmt.Printf("⏱️  Duration: %d seconds\n\n", duration)

	binary := filepath.Join(projectRoot, "bin", "chaos-test-runner")
	if _, err := os.Stat(binary); os.IsNotExist(err) {
		fmt.Println("❌ Error: chaos-test-runner binary not found")
		fmt.Println("   Please build first: go build -o bin/chaos-test-runner ./tests/chaos-test-runner")
		os.Exit(1)
	}

	cmd := exec.Command(binary, "-scenario", scenario, "-duration", fmt.Sprintf("%d", duration))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = projectRoot

	if err := cmd.Run(); err != nil {
		fmt.Printf("❌ Test failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n✅ Test completed successfully!")
	fmt.Printf("📄 Report: chaos-test-report-%s.md\n", scenario)
}

func runAllScenarios(duration int) {
	scenarios := []string{
		"baseline",
		"network-delay",
		"high-network-delay",
		"network-loss",
		"high-network-loss",
		"combined-network",
		"cpu-stress",
		"extreme-cpu-stress",
		"clock-skew",
		"cascade-failure",
	}

	fmt.Printf("🔥 Running all %d scenarios\n", len(scenarios))
	fmt.Printf("⏱️  Duration per scenario: %d seconds\n", duration)
	fmt.Printf("⏱️  Total estimated time: %d minutes\n\n", (duration*len(scenarios))/60)

	timestamp := time.Now().Format("20060102-150405")
	resultsDir := filepath.Join(projectRoot, fmt.Sprintf("chaos-results-%s", timestamp))
	os.MkdirAll(resultsDir, 0755)

	passed := 0
	failed := 0

	for i, scenario := range scenarios {
		fmt.Printf("\n[%d/%d] Testing: %s\n", i+1, len(scenarios), scenario)
		fmt.Println(strings.Repeat("─", 60))

		binary := filepath.Join(projectRoot, "bin", "chaos-test-runner")
		cmd := exec.Command(binary, "-scenario", scenario, "-duration", fmt.Sprintf("%d", duration))
		cmd.Dir = projectRoot

		if err := cmd.Run(); err != nil {
			fmt.Printf("❌ Failed\n")
			failed++
		} else {
			fmt.Printf("✅ Passed\n")
			passed++

			// Move report
			reportFile := fmt.Sprintf("chaos-test-report-%s.md", scenario)
			if _, err := os.Stat(reportFile); err == nil {
				os.Rename(reportFile, filepath.Join(resultsDir, reportFile))
			}
		}
	}

	fmt.Println("\n" + strings.Repeat("═", 60))
	fmt.Println("📊 SUMMARY")
	fmt.Println(strings.Repeat("═", 60))
	fmt.Printf("Total:  %d scenarios\n", len(scenarios))
	fmt.Printf("Passed: %d ✅\n", passed)
	fmt.Printf("Failed: %d ❌\n", failed)
	fmt.Printf("Rate:   %.1f%%\n", float64(passed)/float64(len(scenarios))*100)
	fmt.Println(strings.Repeat("═", 60))
	fmt.Printf("\n📂 Results: %s\n", resultsDir)

	// Generate HTML report
	fmt.Println("\n📊 Generating HTML report...")
	generateReport(resultsDir)
}

func startDashboard(port int) {
	fmt.Printf("🔥 Starting Chaos Engineering Dashboard\n")
	fmt.Printf("📊 Port: %d\n", port)
	fmt.Printf("🌐 URL: http://localhost:%d\n\n", port)

	binary := filepath.Join(projectRoot, "bin", "chaos-dashboard")
	if _, err := os.Stat(binary); os.IsNotExist(err) {
		fmt.Println("❌ Error: chaos-dashboard binary not found")
		fmt.Println("   Please build first: go build -o bin/chaos-dashboard ./cmd/chaos-dashboard")
		os.Exit(1)
	}

	cmd := exec.Command(binary, "-port", fmt.Sprintf("%d", port))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = projectRoot

	if err := cmd.Run(); err != nil {
		fmt.Printf("❌ Dashboard failed: %v\n", err)
		os.Exit(1)
	}
}

func runGameDay() {
	fmt.Println("🔥 Starting Game Day Exercise")
	fmt.Println(strings.Repeat("═", 60))
	fmt.Println("This will run 5 progressive phases:")
	fmt.Println("  1. Warm-up (60s)")
	fmt.Println("  2. Network Resilience (90s)")
	fmt.Println("  3. Resource Pressure (120s)")
	fmt.Println("  4. Combined Failures (120s)")
	fmt.Println("  5. Extreme Conditions (180s)")
	fmt.Println(strings.Repeat("═", 60))
	fmt.Println()

	script := filepath.Join(projectRoot, "scripts", "gameday-runner.sh")
	if _, err := os.Stat(script); os.IsNotExist(err) {
		fmt.Println("❌ Error: gameday-runner.sh not found")
		os.Exit(1)
	}

	cmd := exec.Command("/bin/bash", script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = projectRoot

	if err := cmd.Run(); err != nil {
		fmt.Printf("❌ Game Day failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n✅ Game Day completed successfully!")
}

func generateReport(resultsDir string) {
	fmt.Printf("📊 Generating HTML report for: %s\n", resultsDir)

	script := filepath.Join(projectRoot, "scripts", "generate-html-report.py")
	if _, err := os.Stat(script); os.IsNotExist(err) {
		fmt.Println("❌ Error: generate-html-report.py not found")
		os.Exit(1)
	}

	cmd := exec.Command("python3", script, resultsDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = projectRoot

	if err := cmd.Run(); err != nil {
		fmt.Printf("❌ Report generation failed: %v\n", err)
		os.Exit(1)
	}

	reportPath := filepath.Join(resultsDir, "chaos-test-report.html")
	fmt.Printf("\n✅ Report generated: %s\n", reportPath)
	fmt.Printf("🌐 Open in browser: file://%s\n", reportPath)
}

func createBaseline(resultsDir, outputFile string) {
	fmt.Printf("📊 Creating baseline from: %s\n", resultsDir)

	script := filepath.Join(projectRoot, "scripts", "compare-baseline.py")
	cmd := exec.Command("python3", script, "--create", resultsDir, outputFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = projectRoot

	if err := cmd.Run(); err != nil {
		fmt.Printf("❌ Baseline creation failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n✅ Baseline created: %s\n", outputFile)
}

func compareBaseline(baselineFile, resultsDir string) {
	fmt.Printf("📊 Comparing against baseline: %s\n", baselineFile)

	script := filepath.Join(projectRoot, "scripts", "compare-baseline.py")
	cmd := exec.Command("python3", script, baselineFile, resultsDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = projectRoot

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 {
				fmt.Println("\n⚠️  Performance regressions detected!")
				os.Exit(1)
			}
		}
		fmt.Printf("❌ Comparison failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n✅ No regressions detected!")
}

func listResults() {
	fmt.Println("📂 Recent Test Results")
	fmt.Println(strings.Repeat("═", 60))

	// Find chaos-results directories
	pattern := filepath.Join(projectRoot, "chaos-results-*")
	matches, _ := filepath.Glob(pattern)

	// Find gameday directories
	gamedayPattern := filepath.Join(projectRoot, "gameday-*")
	gamedayMatches, _ := filepath.Glob(gamedayPattern)

	matches = append(matches, gamedayMatches...)

	if len(matches) == 0 {
		fmt.Println("No test results found.")
		return
	}

	for _, dir := range matches {
		info, err := os.Stat(dir)
		if err != nil {
			continue
		}

		// Count markdown reports
		reports, _ := filepath.Glob(filepath.Join(dir, "*.md"))

		fmt.Printf("\n📁 %s\n", filepath.Base(dir))
		fmt.Printf("   Date: %s\n", info.ModTime().Format("2006-01-02 15:04:05"))
		fmt.Printf("   Reports: %d\n", len(reports))

		// Check for HTML report
		htmlReport := filepath.Join(dir, "chaos-test-report.html")
		if _, err := os.Stat(htmlReport); err == nil {
			fmt.Printf("   HTML: ✅\n")
		}
	}

	fmt.Println(strings.Repeat("═", 60))
}

func showStatus() {
	fmt.Println("🔥 EMQX-Go Chaos Engineering System Status")
	fmt.Println(strings.Repeat("═", 60))

	// Check binaries
	fmt.Println("\n📦 Binaries:")
	checkBinary("emqx-go", "EMQX-Go broker")
	checkBinary("chaos-test-runner", "Chaos test runner")
	checkBinary("chaos-dashboard", "Monitoring dashboard")

	// Check scripts
	fmt.Println("\n📜 Scripts:")
	checkScript("advanced-chaos-test.sh", "Advanced test suite")
	checkScript("gameday-runner.sh", "Game Day runner")
	checkScript("generate-html-report.py", "HTML report generator")
	checkScript("compare-baseline.py", "Baseline comparison")
	checkScript("analyze-chaos-results.py", "Results analyzer")

	// Check Python
	fmt.Println("\n🐍 Python:")
	cmd := exec.Command("python3", "--version")
	if output, err := cmd.Output(); err == nil {
		fmt.Printf("   ✅ %s\n", strings.TrimSpace(string(output)))
	} else {
		fmt.Println("   ❌ Not found")
	}

	// System info
	fmt.Println("\n💻 System:")
	fmt.Printf("   Project: %s\n", projectRoot)
	fmt.Printf("   Version: %s\n", version)

	// Count results
	chaosResults, _ := filepath.Glob(filepath.Join(projectRoot, "chaos-results-*"))
	gamedayResults, _ := filepath.Glob(filepath.Join(projectRoot, "gameday-*"))
	fmt.Printf("   Test Results: %d\n", len(chaosResults))
	fmt.Printf("   Game Days: %d\n", len(gamedayResults))

	fmt.Println(strings.Repeat("═", 60))
}

func checkBinary(name, description string) {
	path := filepath.Join(projectRoot, "bin", name)
	if _, err := os.Stat(path); err == nil {
		fmt.Printf("   ✅ %s (%s)\n", name, description)
	} else {
		fmt.Printf("   ❌ %s (%s) - not found\n", name, description)
	}
}

func checkScript(name, description string) {
	path := filepath.Join(projectRoot, "scripts", name)
	if _, err := os.Stat(path); err == nil {
		fmt.Printf("   ✅ %s (%s)\n", name, description)
	} else {
		fmt.Printf("   ❌ %s (%s) - not found\n", name, description)
	}
}

func cleanResults(days int) {
	fmt.Printf("🧹 Cleaning test results older than %d days\n", days)

	cutoff := time.Now().AddDate(0, 0, -days)
	removed := 0

	// Clean chaos-results
	pattern := filepath.Join(projectRoot, "chaos-results-*")
	matches, _ := filepath.Glob(pattern)

	for _, dir := range matches {
		info, err := os.Stat(dir)
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			fmt.Printf("   Removing: %s\n", filepath.Base(dir))
			os.RemoveAll(dir)
			removed++
		}
	}

	// Clean gameday results
	gamedayPattern := filepath.Join(projectRoot, "gameday-*")
	gamedayMatches, _ := filepath.Glob(gamedayPattern)

	for _, dir := range gamedayMatches {
		info, err := os.Stat(dir)
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			fmt.Printf("   Removing: %s\n", filepath.Base(dir))
			os.RemoveAll(dir)
			removed++
		}
	}

	fmt.Printf("\n✅ Removed %d old result directories\n", removed)
}

func main() {
	// Test command flags
	testCmd.Flags().IntP("duration", "d", 30, "Test duration in seconds")

	// Dashboard command flags
	dashboardCmd.Flags().IntP("port", "p", 8888, "Dashboard port")

	// Clean command flags
	cleanCmd.Flags().IntP("days", "d", 30, "Remove results older than N days")

	// Add subcommands to baseline
	baselineCmd.AddCommand(baselineCreateCmd)
	baselineCmd.AddCommand(baselineCompareCmd)

	// Add commands to root
	rootCmd.AddCommand(testCmd)
	rootCmd.AddCommand(dashboardCmd)
	rootCmd.AddCommand(gamedayCmd)
	rootCmd.AddCommand(reportCmd)
	rootCmd.AddCommand(baselineCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(cleanCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
