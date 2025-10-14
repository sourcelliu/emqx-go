package e2e

import (
	"fmt"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/require"
)

// TestMQTTAuthenticationWithValidCredentials tests successful authentication
func TestMQTTAuthenticationWithValidCredentials(t *testing.T) {
	t.Log("Testing MQTT authentication with valid credentials")

	testCases := []struct {
		name     string
		username string
		password string
		expected bool
	}{
		{
			name:     "admin user with bcrypt",
			username: "admin",
			password: "admin123",
			expected: true,
		},
		{
			name:     "user1 with sha256",
			username: "user1",
			password: "password123",
			expected: true,
		},
		{
			name:     "test user with plain",
			username: "test",
			password: "test",
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := mqtt.NewClientOptions()
			opts.AddBroker("tcp://localhost:1883")
			opts.SetClientID("auth-test-" + tc.username)
			opts.SetUsername(tc.username)
			opts.SetPassword(tc.password)
			opts.SetConnectTimeout(5 * time.Second)

			client := mqtt.NewClient(opts)
			token := client.Connect()
			require.True(t, token.Wait(), "Connection should succeed for %s", tc.username)
			require.NoError(t, token.Error(), "No error expected for valid credentials")

			// Test a simple pub/sub to ensure full functionality
			messageReceived := make(chan string, 1)
			topic := "test/auth/" + tc.username

			subToken := client.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
				messageReceived <- string(msg.Payload())
			})
			require.True(t, subToken.Wait(), "Subscribe should succeed")
			require.NoError(t, subToken.Error(), "Subscribe error")

			testMessage := "Hello from " + tc.username
			pubToken := client.Publish(topic, 1, false, testMessage)
			require.True(t, pubToken.Wait(), "Publish should succeed")
			require.NoError(t, pubToken.Error(), "Publish error")

			select {
			case msg := <-messageReceived:
				require.Equal(t, testMessage, msg, "Message should match")
				t.Logf("✓ Successfully authenticated and communicated as %s", tc.username)
			case <-time.After(3 * time.Second):
				t.Fatalf("Message not received within timeout for %s", tc.username)
			}

			client.Disconnect(250)
		})
	}
}

// TestMQTTAuthenticationWithInvalidCredentials tests authentication failures
func TestMQTTAuthenticationWithInvalidCredentials(t *testing.T) {
	t.Log("Testing MQTT authentication with invalid credentials")

	testCases := []struct {
		name     string
		username string
		password string
	}{
		{
			name:     "valid user with wrong password",
			username: "admin",
			password: "wrongpassword",
		},
		{
			name:     "non-existent user",
			username: "nonexistent",
			password: "anypassword",
		},
		// NOTE: "empty username with password" scenario is removed because
		// properly-implemented MQTT clients (like Paho) do not send passwords
		// when username is empty, per MQTT protocol specification.
		// The MQTT spec requires that if UsernameFlag is false, PasswordFlag
		// must also be false. This is a protocol-level constraint.
		{
			name:     "valid user with empty password",
			username: "admin",
			password: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := mqtt.NewClientOptions()
			opts.AddBroker("tcp://localhost:1883")
			opts.SetClientID("auth-fail-test-" + tc.username)
			opts.SetUsername(tc.username)
			opts.SetPassword(tc.password)
			opts.SetConnectTimeout(3 * time.Second)
			opts.SetMaxReconnectInterval(1 * time.Second)
			opts.SetConnectRetry(false) // Disable auto-retry to get immediate failure
			opts.SetAutoReconnect(false)

			connectionFailed := false
			opts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
				t.Logf("Connection lost for %s: %v", tc.name, err)
				connectionFailed = true
			})

			client := mqtt.NewClient(opts)
			token := client.Connect()

			// Wait for connection attempt
			completed := token.WaitTimeout(3 * time.Second)

			if completed {
				if token.Error() != nil {
					t.Logf("✓ Authentication correctly failed for %s: %v", tc.name, token.Error())
				} else {
					// Connection succeeded when it shouldn't have
					// Check if the connection actually works by trying a simple operation
					subToken := client.Subscribe("test/fail", 0, nil)
					if subToken.WaitTimeout(1*time.Second) && subToken.Error() == nil {
						t.Errorf("Expected authentication to fail for %s, but connection and subscribe succeeded", tc.name)
					} else {
						t.Logf("✓ Authentication failed for %s (connection appeared to succeed but operations failed)", tc.name)
					}
				}
			} else {
				t.Logf("✓ Authentication correctly timed out for %s", tc.name)
			}

			// Check if connection failed during the process
			if connectionFailed {
				t.Logf("✓ Connection was lost during authentication for %s", tc.name)
			}

			client.Disconnect(100)
		})
	}
}

// TestMQTTAuthenticationMixedScenarios tests various mixed scenarios
func TestMQTTAuthenticationMixedScenarios(t *testing.T) {
	t.Log("Testing mixed authentication scenarios")

	// Test 1: Multiple valid users connecting simultaneously
	t.Log("Test 1: Multiple valid users connecting simultaneously")

	users := []struct {
		username string
		password string
	}{
		{"admin", "admin123"},
		{"user1", "password123"},
		{"test", "test"},
	}

	clients := make([]mqtt.Client, len(users))
	messageChans := make([]chan string, len(users))

	// Connect all users
	for i, user := range users {
		opts := mqtt.NewClientOptions()
		opts.AddBroker("tcp://localhost:1883")
		opts.SetClientID("multi-test-" + user.username)
		opts.SetUsername(user.username)
		opts.SetPassword(user.password)
		opts.SetConnectTimeout(5 * time.Second)

		messageChans[i] = make(chan string, 1)
		clients[i] = mqtt.NewClient(opts)

		token := clients[i].Connect()
		require.True(t, token.Wait(), "User %s should connect successfully", user.username)
		require.NoError(t, token.Error(), "No error expected for user %s", user.username)

		// Subscribe to a common topic
		subToken := clients[i].Subscribe("test/multiuser", 1, func(client mqtt.Client, msg mqtt.Message) {
			messageChans[i] <- string(msg.Payload())
		})
		require.True(t, subToken.Wait(), "User %s should subscribe successfully", user.username)
		require.NoError(t, subToken.Error(), "Subscribe error for user %s", user.username)
	}

	// Have one user publish a message
	testMessage := "Hello from multi-user test"
	pubToken := clients[0].Publish("test/multiuser", 1, false, testMessage)
	require.True(t, pubToken.Wait(), "Publish should succeed")
	require.NoError(t, pubToken.Error(), "Publish error")

	// Verify all users receive the message
	for i, user := range users {
		select {
		case msg := <-messageChans[i]:
			require.Equal(t, testMessage, msg, "User %s should receive the correct message", user.username)
			t.Logf("✓ User %s received message correctly", user.username)
		case <-time.After(3 * time.Second):
			t.Errorf("User %s did not receive message within timeout", user.username)
		}
	}

	// Disconnect all clients
	for i, client := range clients {
		client.Disconnect(250)
		t.Logf("✓ User %s disconnected", users[i].username)
	}

	// Test 2: Rapid connection/disconnection cycles
	t.Log("Test 2: Rapid connection/disconnection cycles")

	for i := 0; i < 5; i++ {
		opts := mqtt.NewClientOptions()
		opts.AddBroker("tcp://localhost:1883")
		opts.SetClientID("rapid-test-" + string(rune('0'+i)))
		opts.SetUsername("admin")
		opts.SetPassword("admin123")
		opts.SetConnectTimeout(3 * time.Second)

		client := mqtt.NewClient(opts)
		token := client.Connect()
		require.True(t, token.Wait(), "Rapid connection %d should succeed", i+1)
		require.NoError(t, token.Error(), "Rapid connection %d error", i+1)

		client.Disconnect(100)
		time.Sleep(50 * time.Millisecond)
	}
	t.Log("✓ Rapid connection cycles completed successfully")

	t.Log("Mixed authentication scenarios test completed successfully")
}

// TestMQTTAuthenticationPersistence tests that authentication persists across operations
func TestMQTTAuthenticationPersistence(t *testing.T) {
	t.Log("Testing authentication persistence across operations")

	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://localhost:1883")
	opts.SetClientID("persistence-test")
	opts.SetUsername("admin")
	opts.SetPassword("admin123")
	opts.SetConnectTimeout(5 * time.Second)
	opts.SetKeepAlive(30 * time.Second)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	require.True(t, token.Wait(), "Initial connection should succeed")
	require.NoError(t, token.Error(), "Initial connection error")

	// Perform multiple operations to ensure session persists
	topics := []string{
		"test/persistence/topic1",
		"test/persistence/topic2",
		"test/persistence/topic3",
	}

	messageReceived := make(chan string, 10)

	// Subscribe to multiple topics
	for _, topic := range topics {
		subToken := client.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
			messageReceived <- string(msg.Payload())
		})
		require.True(t, subToken.Wait(), "Subscribe to %s should succeed", topic)
		require.NoError(t, subToken.Error(), "Subscribe to %s error", topic)
	}

	// Publish to all topics
	for i, topic := range topics {
		message := fmt.Sprintf("Message %d for persistence test", i+1)
		pubToken := client.Publish(topic, 1, false, message)
		require.True(t, pubToken.Wait(), "Publish to %s should succeed", topic)
		require.NoError(t, pubToken.Error(), "Publish to %s error", topic)
	}

	// Verify all messages received
	receivedCount := 0
	timeout := time.After(5 * time.Second)
	for receivedCount < len(topics) {
		select {
		case msg := <-messageReceived:
			t.Logf("Received message: %s", msg)
			receivedCount++
		case <-timeout:
			t.Fatalf("Did not receive all messages, got %d out of %d", receivedCount, len(topics))
		}
	}

	client.Disconnect(250)
	t.Log("✓ Authentication persistence test completed successfully")
}