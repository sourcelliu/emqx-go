// Copyright 2023 The emqx-go Authors
// Simple dashboard starter for testing

package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/turtacn/emqx-go/pkg/admin"
	"github.com/turtacn/emqx-go/pkg/blacklist"
	"github.com/turtacn/emqx-go/pkg/broker"
	"github.com/turtacn/emqx-go/pkg/connector"
	"github.com/turtacn/emqx-go/pkg/dashboard"
	"github.com/turtacn/emqx-go/pkg/integration"
	"github.com/turtacn/emqx-go/pkg/metrics"
	"github.com/turtacn/emqx-go/pkg/monitor"
	"github.com/turtacn/emqx-go/pkg/rules"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log.Println("Starting EMQX-Go with Dashboard...")

	// Start broker
	brokerInstance := broker.New("emqx-go-with-dashboard", nil)
	brokerInstance.SetupDefaultAuth()
	defer brokerInstance.Close()

	go brokerInstance.StartServer(ctx, ":1883")
	time.Sleep(200 * time.Millisecond)

	// Start metrics server
	go metrics.Serve(":8082")
	time.Sleep(100 * time.Millisecond)

	// Create dashboard components
	metricsManager := metrics.NewMetricsManager()
	healthChecker := monitor.NewHealthChecker()

	// Create connector manager
	connectorManager := connector.NewConnectorManager()

	// Get the rule engine from the broker (they share the same instance)
	ruleEngine := brokerInstance.GetRuleEngine()

	// Create integration engine
	integrationEngine := integration.NewDataIntegrationEngine()

	// Set up republish callback for rule engine
	republishExecutor, err := ruleEngine.GetActionExecutor(rules.ActionTypeRepublish)
	if err == nil {
		if repubExecutor, ok := republishExecutor.(*rules.RepublishActionExecutor); ok {
			repubExecutor.SetRepublishCallback(func(topic string, qos int, payload []byte) error {
				brokerInstance.RouteToLocalSubscribersWithQoS(topic, payload, byte(qos))
				return nil
			})
		}
	}

	// Create a real broker interface instead of mock for production use
	brokerInterface := &realBrokerInterface{broker: brokerInstance}
	adminAPI := admin.NewAPIServer(metricsManager, brokerInterface)

	// Create dashboard server with default config
	config := dashboard.DefaultConfig()
	config.Address = "127.0.0.1"  // Bind to localhost for security

	dashboardServer, err := dashboard.NewServer(config, adminAPI, metricsManager, healthChecker, brokerInstance.GetCertificateManager(), connectorManager, ruleEngine, integrationEngine)
	if err != nil {
		log.Fatalf("Failed to create dashboard server: %v", err)
	}

	// Start dashboard server
	go func() {
		if err := dashboardServer.Start(ctx); err != nil {
			log.Printf("Dashboard server error: %v", err)
		}
	}()

	log.Printf("✅ EMQX-Go services started successfully!")
	log.Printf("📊 Dashboard: http://localhost:18083")
	log.Printf("🔐 Username: admin")
	log.Printf("🔑 Password: public")
	log.Printf("🚀 MQTT Broker: mqtt://localhost:1883")
	log.Printf("📈 Metrics: http://localhost:8082/metrics")
	log.Printf("⚡ Rule Engine: Enabled with Dashboard Management")

	// Add some default test rules for demonstration
	addDefaultTestRules(ruleEngine)

	log.Printf("Press Ctrl+C to stop...")

	// Wait for termination signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
	cancel()
	dashboardServer.Stop()

	log.Println("Services stopped.")
}

// addDefaultTestRules adds some demo rules for testing
func addDefaultTestRules(ruleEngine *rules.RuleEngine) {
	// Rule 1: Console action for temperature sensor
	temperatureRule := rules.Rule{
		ID:          "demo-temperature-console",
		Name:        "Temperature Console Logger",
		Description: "Log temperature sensor data to console",
		SQL:         "topic = 'sensor/temperature'",
		Actions: []rules.Action{
			{
				Type: rules.ActionTypeConsole,
				Parameters: map[string]interface{}{
					"level":    "info",
					"template": "🌡️  Temperature sensor data: ${payload} from client ${clientid}",
				},
			},
		},
		Status:   rules.RuleStatusEnabled,
		Priority: 1,
	}

	// Rule 2: Console action for any sensor topic
	sensorRule := rules.Rule{
		ID:          "demo-sensor-console",
		Name:        "All Sensors Console Logger",
		Description: "Log all sensor data to console",
		SQL:         "topic LIKE 'sensor/%'",
		Actions: []rules.Action{
			{
				Type: rules.ActionTypeConsole,
				Parameters: map[string]interface{}{
					"level":    "debug",
					"template": "📊 Sensor data on ${topic}: ${payload}",
				},
			},
		},
		Status:   rules.RuleStatusEnabled,
		Priority: 2,
	}

	// Add the rules
	if err := ruleEngine.CreateRule(temperatureRule); err != nil {
		log.Printf("[WARN] Failed to create temperature rule: %v", err)
	} else {
		log.Printf("[INFO] Created demo rule: %s", temperatureRule.ID)
	}

	if err := ruleEngine.CreateRule(sensorRule); err != nil {
		log.Printf("[WARN] Failed to create sensor rule: %v", err)
	} else {
		log.Printf("[INFO] Created demo rule: %s", sensorRule.ID)
	}
}

// Simple broker interface implementation for demo
type realBrokerInterface struct {
	broker *broker.Broker
}

func (r *realBrokerInterface) GetConnections() []admin.ConnectionInfo {
	// For demo purposes, return some mock data
	// In a real implementation, this would query the actual broker
	return []admin.ConnectionInfo{}
}

func (r *realBrokerInterface) GetSessions() []admin.SessionInfo {
	return []admin.SessionInfo{}
}

func (r *realBrokerInterface) GetSubscriptions() []admin.SubscriptionInfo {
	return []admin.SubscriptionInfo{}
}

func (r *realBrokerInterface) GetRoutes() []admin.RouteInfo {
	return []admin.RouteInfo{}
}

func (r *realBrokerInterface) GetClusterNodes() []admin.NodeInfo {
	return []admin.NodeInfo{
		{
			Node:       "emqx-go-with-dashboard",
			NodeStatus: "running",
			Version:    "1.0.0",
			Uptime:     time.Now().Unix(),
			Datetime:   time.Now(),
		},
	}
}

func (r *realBrokerInterface) DisconnectClient(clientID string) error {
	// Implementation would disconnect the actual client
	return nil
}

func (r *realBrokerInterface) KickoutSession(clientID string) error {
	// Implementation would kickout the actual session
	return nil
}

func (r *realBrokerInterface) GetNodeInfo() admin.NodeInfo {
	return admin.NodeInfo{
		Node:       "emqx-go-with-dashboard",
		NodeStatus: "running",
		Version:    "1.0.0",
		Uptime:     time.Now().Unix(),
		Datetime:   time.Now(),
	}
}

func (r *realBrokerInterface) GetBlacklistMiddleware() *blacklist.BlacklistMiddleware {
	if r.broker != nil {
		return r.broker.GetBlacklistMiddleware()
	}
	return nil
}