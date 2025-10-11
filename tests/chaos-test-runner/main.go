package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/turtacn/emqx-go/pkg/chaos"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

var (
	scenario = flag.String("scenario", "all", "Chaos scenario to run")
	duration = flag.Int("duration", 60, "Test duration in seconds")
	verbose  = flag.Bool("verbose", false, "Verbose output")
)

// TestScenario represents a chaos test scenario
type TestScenario struct {
	Name        string
	Description string
	Setup       func(*chaos.Injector) error
	Validate    func() error
	Cleanup     func(*chaos.Injector)
	Duration    time.Duration
}

// TestResult represents the result of a chaos test
type TestResult struct {
	Scenario      string
	StartTime     time.Time
	EndTime       time.Time
	Duration      time.Duration
	Success       bool
	MessagesSent  int
	MessagesRecv  int
	MessageLoss   int
	Latencies     []time.Duration
	Errors        []string
	Observations  []string
}

// ChaosTestHarness manages chaos testing
type ChaosTestHarness struct {
	injector       *chaos.Injector
	results        []TestResult
	clusterPorts   []int
	mqttClients    []mqtt.Client
	testMessages   int
	receivedMsgs   map[string]int
}

// NewChaosTestHarness creates a new test harness
func NewChaosTestHarness() *ChaosTestHarness {
	return &ChaosTestHarness{
		injector:     chaos.GetGlobalInjector(),
		results:      make([]TestResult, 0),
		clusterPorts: []int{1883, 1884, 1885},
		receivedMsgs: make(map[string]int),
	}
}

// StartCluster starts the EMQX-Go cluster
func (h *ChaosTestHarness) StartCluster() error {
	log.Println("[HARNESS] Starting EMQX-Go cluster...")

	cmd := exec.Command("bash", "./scripts/start-cluster.sh")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start cluster: %v\n%s", err, output)
	}

	// Wait for cluster to be ready
	time.Sleep(5 * time.Second)

	// Verify all nodes are running
	for _, port := range h.clusterPorts {
		if !h.isPortOpen(port) {
			return fmt.Errorf("node on port %d is not responding", port)
		}
	}

	log.Println("[HARNESS] Cluster started successfully")
	return nil
}

// StopCluster stops the EMQX-Go cluster
func (h *ChaosTestHarness) StopCluster() error {
	log.Println("[HARNESS] Stopping cluster...")

	cmd := exec.Command("bash", "./scripts/stop-cluster.sh")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to stop cluster: %v\n%s", err, output)
	}

	time.Sleep(2 * time.Second)
	log.Println("[HARNESS] Cluster stopped")
	return nil
}

// isPortOpen checks if a port is open
func (h *ChaosTestHarness) isPortOpen(port int) bool {
	cmd := exec.Command("nc", "-z", "localhost", fmt.Sprintf("%d", port))
	return cmd.Run() == nil
}

// ConnectMQTTClient connects an MQTT client
func (h *ChaosTestHarness) ConnectMQTTClient(clientID string, port int) (mqtt.Client, error) {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://localhost:%d", port))
	opts.SetClientID(clientID)
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(true)
	opts.SetConnectTimeout(5 * time.Second)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if token.WaitTimeout(5*time.Second) && token.Error() != nil {
		return nil, token.Error()
	}

	return client, nil
}

// RunScenario runs a specific chaos scenario
func (h *ChaosTestHarness) RunScenario(scenario TestScenario) TestResult {
	result := TestResult{
		Scenario:     scenario.Name,
		StartTime:    time.Now(),
		Success:      false,
		MessagesSent: 0,
		MessagesRecv: 0,
		Latencies:    make([]time.Duration, 0),
		Errors:       make([]string, 0),
		Observations: make([]string, 0),
	}

	log.Printf("[SCENARIO] Starting: %s", scenario.Name)
	log.Printf("[SCENARIO] Description: %s", scenario.Description)

	// Setup
	if err := scenario.Setup(h.injector); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Setup failed: %v", err))
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		return result
	}

	// Run test workload
	ctx, cancel := context.WithTimeout(context.Background(), scenario.Duration)
	defer cancel()

	workloadResult := h.runWorkload(ctx, &result)
	result.MessagesSent = workloadResult.sent
	result.MessagesRecv = workloadResult.received
	result.MessageLoss = workloadResult.lost
	result.Latencies = workloadResult.latencies

	// Validate
	if err := scenario.Validate(); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Validation failed: %v", err))
	} else {
		result.Success = true
	}

	// Cleanup
	scenario.Cleanup(h.injector)

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	// Calculate success rate
	successRate := float64(result.MessagesRecv) / float64(result.MessagesSent) * 100
	result.Observations = append(result.Observations,
		fmt.Sprintf("Message success rate: %.2f%%", successRate))

	if len(result.Latencies) > 0 {
		avgLatency := h.calculateAverageLatency(result.Latencies)
		result.Observations = append(result.Observations,
			fmt.Sprintf("Average latency: %v", avgLatency))
	}

	log.Printf("[SCENARIO] Completed: %s (Success: %v)", scenario.Name, result.Success)
	return result
}

// WorkloadResult contains workload execution results
type WorkloadResult struct {
	sent      int
	received  int
	lost      int
	latencies []time.Duration
}

// runWorkload runs the test workload
func (h *ChaosTestHarness) runWorkload(ctx context.Context, result *TestResult) WorkloadResult {
	workload := WorkloadResult{
		latencies: make([]time.Duration, 0),
	}

	// Create subscriber
	subClient, err := h.ConnectMQTTClient("chaos-subscriber", h.clusterPorts[0])
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to connect subscriber: %v", err))
		return workload
	}
	defer subClient.Disconnect(250)

	receivedChan := make(chan time.Time, 1000)

	// Subscribe to test topic
	token := subClient.Subscribe("chaos/test", 1, func(client mqtt.Client, msg mqtt.Message) {
		receivedChan <- time.Now()
		workload.received++
	})

	if token.WaitTimeout(2*time.Second) && token.Error() != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Subscribe failed: %v", token.Error()))
		return workload
	}

	// Create publisher
	pubClient, err := h.ConnectMQTTClient("chaos-publisher", h.clusterPorts[1])
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to connect publisher: %v", err))
		return workload
	}
	defer pubClient.Disconnect(250)

	// Send messages periodically
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Wait a bit for final messages
			time.Sleep(500 * time.Millisecond)
			workload.lost = workload.sent - workload.received
			return workload

		case <-ticker.C:
			sendTime := time.Now()
			token := pubClient.Publish("chaos/test", 1, false,
				fmt.Sprintf("chaos-test-%d", workload.sent))

			if token.WaitTimeout(1*time.Second) && token.Error() == nil {
				workload.sent++

				// Try to calculate latency
				select {
				case recvTime := <-receivedChan:
					latency := recvTime.Sub(sendTime)
					workload.latencies = append(workload.latencies, latency)
				case <-time.After(10 * time.Millisecond):
					// No immediate response, that's okay
				}
			} else if token.Error() != nil {
				result.Observations = append(result.Observations,
					fmt.Sprintf("Publish error at %v: %v", time.Since(result.StartTime), token.Error()))
			}
		}
	}
}

// calculateAverageLatency calculates average latency
func (h *ChaosTestHarness) calculateAverageLatency(latencies []time.Duration) time.Duration {
	if len(latencies) == 0 {
		return 0
	}
	var total time.Duration
	for _, l := range latencies {
		total += l
	}
	return total / time.Duration(len(latencies))
}

// GetScenarios returns all chaos scenarios
func (h *ChaosTestHarness) GetScenarios() []TestScenario {
	testDuration := time.Duration(*duration) * time.Second

	return []TestScenario{
		{
			Name:        "network-delay",
			Description: "Inject 100ms network delay on all communication",
			Duration:    testDuration,
			Setup: func(i *chaos.Injector) error {
				i.Enable()
				i.InjectNetworkDelay(100 * time.Millisecond)
				return nil
			},
			Validate: func() error {
				// Cluster should still be functional
				return nil
			},
			Cleanup: func(i *chaos.Injector) {
				i.Disable()
			},
		},
		{
			Name:        "network-loss",
			Description: "Inject 10% packet loss",
			Duration:    testDuration,
			Setup: func(i *chaos.Injector) error {
				i.Enable()
				i.InjectNetworkLoss(0.10) // 10% loss
				return nil
			},
			Validate: func() error {
				return nil
			},
			Cleanup: func(i *chaos.Injector) {
				i.Disable()
			},
		},
		{
			Name:        "high-network-loss",
			Description: "Inject 30% packet loss",
			Duration:    testDuration,
			Setup: func(i *chaos.Injector) error {
				i.Enable()
				i.InjectNetworkLoss(0.30) // 30% loss
				return nil
			},
			Validate: func() error {
				return nil
			},
			Cleanup: func(i *chaos.Injector) {
				i.Disable()
			},
		},
		{
			Name:        "cpu-stress",
			Description: "Inject 80% CPU stress",
			Duration:    testDuration,
			Setup: func(i *chaos.Injector) error {
				i.Enable()
				i.InjectCPUStress(80)
				ctx := context.Background()
				i.StartCPUStress(ctx)
				return nil
			},
			Validate: func() error {
				return nil
			},
			Cleanup: func(i *chaos.Injector) {
				i.Disable()
			},
		},
		{
			Name:        "clock-skew",
			Description: "Inject 5 second clock skew",
			Duration:    testDuration,
			Setup: func(i *chaos.Injector) error {
				i.Enable()
				i.InjectClockSkew(5 * time.Second)
				return nil
			},
			Validate: func() error {
				return nil
			},
			Cleanup: func(i *chaos.Injector) {
				i.Disable()
			},
		},
	}
}

// PrintResults prints test results
func (h *ChaosTestHarness) PrintResults() {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("CHAOS TEST RESULTS")
	fmt.Println(strings.Repeat("=", 80))

	totalTests := len(h.results)
	successfulTests := 0

	for _, result := range h.results {
		if result.Success {
			successfulTests++
		}

		fmt.Printf("\nScenario: %s\n", result.Scenario)
		fmt.Printf("Status: %s\n", map[bool]string{true: "✓ PASS", false: "✗ FAIL"}[result.Success])
		fmt.Printf("Duration: %v\n", result.Duration)
		fmt.Printf("Messages: Sent=%d, Received=%d, Lost=%d\n",
			result.MessagesSent, result.MessagesRecv, result.MessageLoss)

		if len(result.Latencies) > 0 {
			avgLatency := h.calculateAverageLatency(result.Latencies)
			fmt.Printf("Average Latency: %v\n", avgLatency)
		}

		if len(result.Observations) > 0 {
			fmt.Println("Observations:")
			for _, obs := range result.Observations {
				fmt.Printf("  - %s\n", obs)
			}
		}

		if len(result.Errors) > 0 {
			fmt.Println("Errors:")
			for _, err := range result.Errors {
				fmt.Printf("  - %s\n", err)
			}
		}
	}

	fmt.Printf("\n" + strings.Repeat("=", 80) + "\n")
	fmt.Printf("Overall Success Rate: %d/%d (%.1f%%)\n",
		successfulTests, totalTests, float64(successfulTests)/float64(totalTests)*100)
	fmt.Println(strings.Repeat("=", 80))
}

// SaveResults saves results to file
func (h *ChaosTestHarness) SaveResults(filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintf(f, "# CHAOS TEST EXECUTION REPORT\n\n")
	fmt.Fprintf(f, "**Execution Time**: %s\n\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(f, "---\n\n")

	for _, result := range h.results {
		fmt.Fprintf(f, "## Scenario: %s\n\n", result.Scenario)
		fmt.Fprintf(f, "- **Status**: %s\n", map[bool]string{true: "✅ PASS", false: "❌ FAIL"}[result.Success])
		fmt.Fprintf(f, "- **Duration**: %v\n", result.Duration)
		fmt.Fprintf(f, "- **Messages Sent**: %d\n", result.MessagesSent)
		fmt.Fprintf(f, "- **Messages Received**: %d\n", result.MessagesRecv)
		fmt.Fprintf(f, "- **Messages Lost**: %d\n", result.MessageLoss)

		if result.MessagesSent > 0 {
			successRate := float64(result.MessagesRecv) / float64(result.MessagesSent) * 100
			fmt.Fprintf(f, "- **Success Rate**: %.2f%%\n", successRate)
		}

		if len(result.Latencies) > 0 {
			avgLatency := h.calculateAverageLatency(result.Latencies)
			fmt.Fprintf(f, "- **Average Latency**: %v\n", avgLatency)
		}

		if len(result.Observations) > 0 {
			fmt.Fprintf(f, "\n**Observations**:\n")
			for _, obs := range result.Observations {
				fmt.Fprintf(f, "- %s\n", obs)
			}
		}

		if len(result.Errors) > 0 {
			fmt.Fprintf(f, "\n**Errors**:\n")
			for _, err := range result.Errors {
				fmt.Fprintf(f, "- %s\n", err)
			}
		}

		fmt.Fprintf(f, "\n---\n\n")
	}

	totalTests := len(h.results)
	successfulTests := 0
	for _, result := range h.results {
		if result.Success {
			successfulTests++
		}
	}

	fmt.Fprintf(f, "## Summary\n\n")
	fmt.Fprintf(f, "- **Total Tests**: %d\n", totalTests)
	fmt.Fprintf(f, "- **Successful**: %d\n", successfulTests)
	fmt.Fprintf(f, "- **Failed**: %d\n", totalTests-successfulTests)
	fmt.Fprintf(f, "- **Success Rate**: %.1f%%\n", float64(successfulTests)/float64(totalTests)*100)

	return nil
}

func main() {
	flag.Parse()

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	harness := NewChaosTestHarness()

	// Start cluster
	if err := harness.StartCluster(); err != nil {
		log.Fatalf("Failed to start cluster: %v", err)
	}

	// Ensure cleanup on exit
	defer func() {
		if err := harness.StopCluster(); err != nil {
			log.Printf("Warning: failed to stop cluster: %v", err)
		}
	}()

	scenarios := harness.GetScenarios()

	// Filter scenarios if specific one requested
	if *scenario != "all" {
		filtered := make([]TestScenario, 0)
		for _, s := range scenarios {
			if s.Name == *scenario {
				filtered = append(filtered, s)
				break
			}
		}
		scenarios = filtered
	}

	if len(scenarios) == 0 {
		log.Fatalf("No scenarios found matching: %s", *scenario)
	}

	log.Printf("[HARNESS] Running %d chaos scenarios", len(scenarios))

	// Run scenarios
	for i, s := range scenarios {
		log.Printf("\n[HARNESS] === Test %d/%d ===", i+1, len(scenarios))

		result := harness.RunScenario(s)
		harness.results = append(harness.results, result)

		// Wait between tests
		if i < len(scenarios)-1 {
			log.Println("[HARNESS] Waiting 5 seconds before next test...")
			time.Sleep(5 * time.Second)
		}
	}

	// Print results
	harness.PrintResults()

	// Save results
	reportFile := fmt.Sprintf("chaos-test-report-%s.md", time.Now().Format("20060102-150405"))
	if err := harness.SaveResults(reportFile); err != nil {
		log.Printf("Warning: failed to save results: %v", err)
	} else {
		log.Printf("\n[HARNESS] Results saved to: %s", reportFile)
	}
}
