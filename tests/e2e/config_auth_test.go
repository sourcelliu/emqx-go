package e2e

import (
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/require"
)

// TestMQTTConfigurationBasedAuth tests authentication using users from config file
func TestMQTTConfigurationBasedAuth(t *testing.T) {
	t.Log("Testing MQTT authentication with configuration-based users")

	// Test users from config.yaml
	testCases := []struct {
		name           string
		username       string
		password       string
		expectedResult bool
		description    string
	}{
		{
			name:           "config admin user",
			username:       "admin",
			password:       "admin123",
			expectedResult: true,
			description:    "Config admin user with bcrypt",
		},
		{
			name:           "config user1",
			username:       "user1",
			password:       "password123",
			expectedResult: true,
			description:    "Config user1 with sha256",
		},
		{
			name:           "config test user",
			username:       "test",
			password:       "test",
			expectedResult: true,
			description:    "Config test user with plain",
		},
		{
			name:           "config device001",
			username:       "device001",
			password:       "device_secret_key",
			expectedResult: true,
			description:    "Config IoT device user",
		},
		{
			name:           "disabled guest user",
			username:       "guest",
			password:       "guest123",
			expectedResult: false,
			description:    "Disabled user should fail auth",
		},
		{
			name:           "disabled newuser",
			username:       "newuser",
			password:       "updatedsecret",
			expectedResult: false,
			description:    "CLI-added but disabled user should fail auth",
		},
		{
			name:           "wrong password for valid user",
			username:       "admin",
			password:       "wrongpassword",
			expectedResult: false,
			description:    "Valid user with wrong password should fail",
		},
		{
			name:           "non-existent user",
			username:       "nonexistent",
			password:       "anypassword",
			expectedResult: false,
			description:    "Non-existent user should fail auth",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("Testing: %s", tc.description)

			opts := mqtt.NewClientOptions()
			opts.AddBroker("tcp://localhost:1883")
			opts.SetClientID("config-auth-test-" + tc.username)
			opts.SetUsername(tc.username)
			opts.SetPassword(tc.password)
			opts.SetConnectTimeout(3 * time.Second)
			opts.SetAutoReconnect(false)
			opts.SetConnectRetry(false)

			client := mqtt.NewClient(opts)
			token := client.Connect()

			if tc.expectedResult {
				// Should succeed
				require.True(t, token.Wait(), "Connection should succeed for %s", tc.username)
				require.NoError(t, token.Error(), "No error expected for valid user %s", tc.username)

				// Test basic functionality
				messageReceived := make(chan string, 1)
				topic := "config/test/" + tc.username

				subToken := client.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
					messageReceived <- string(msg.Payload())
				})
				require.True(t, subToken.Wait(), "Subscribe should succeed for %s", tc.username)
				require.NoError(t, subToken.Error(), "Subscribe error for %s", tc.username)

				testMessage := "Config test message for " + tc.username
				pubToken := client.Publish(topic, 1, false, testMessage)
				require.True(t, pubToken.Wait(), "Publish should succeed for %s", tc.username)
				require.NoError(t, pubToken.Error(), "Publish error for %s", tc.username)

				select {
				case msg := <-messageReceived:
					require.Equal(t, testMessage, msg, "Message should match for %s", tc.username)
					t.Logf("✓ Successfully authenticated and communicated as %s", tc.username)
				case <-time.After(3 * time.Second):
					t.Fatalf("Message not received within timeout for %s", tc.username)
				}

				client.Disconnect(250)
			} else {
				// Should fail
				if token.WaitTimeout(3 * time.Second) {
					if token.Error() == nil {
						// Connection appeared to succeed, check if it actually works
						subToken := client.Subscribe("test/fail", 0, nil)
						if subToken.WaitTimeout(1*time.Second) && subToken.Error() == nil {
							t.Errorf("Expected authentication to fail for %s, but connection and operations succeeded", tc.username)
						} else {
							t.Logf("✓ Authentication correctly failed for %s (operations failed)", tc.username)
						}
					} else {
						t.Logf("✓ Authentication correctly failed for %s: %v", tc.username, token.Error())
					}
				} else {
					t.Logf("✓ Authentication correctly timed out for %s", tc.username)
				}

				client.Disconnect(100)
			}
		})
	}
}

// TestMQTTConfigurationMultipleUsers tests multiple users connecting simultaneously
func TestMQTTConfigurationMultipleUsers(t *testing.T) {
	t.Log("Testing multiple configuration-based users connecting simultaneously")

	// Valid users from config
	users := []struct {
		username string
		password string
	}{
		{"admin", "admin123"},
		{"user1", "password123"},
		{"test", "test"},
		{"device001", "device_secret_key"},
	}

	clients := make([]mqtt.Client, len(users))
	messageChans := make([]chan string, len(users))

	// Connect all users
	for i, user := range users {
		opts := mqtt.NewClientOptions()
		opts.AddBroker("tcp://localhost:1883")
		opts.SetClientID("config-multi-test-" + user.username)
		opts.SetUsername(user.username)
		opts.SetPassword(user.password)
		opts.SetConnectTimeout(5 * time.Second)

		messageChans[i] = make(chan string, 1)
		clients[i] = mqtt.NewClient(opts)

		token := clients[i].Connect()
		require.True(t, token.Wait(), "User %s should connect successfully", user.username)
		require.NoError(t, token.Error(), "No error expected for user %s", user.username)

		// Subscribe to a common topic
		subToken := clients[i].Subscribe("config/multiuser", 1, func(client mqtt.Client, msg mqtt.Message) {
			messageChans[i] <- string(msg.Payload())
		})
		require.True(t, subToken.Wait(), "User %s should subscribe successfully", user.username)
		require.NoError(t, subToken.Error(), "Subscribe error for user %s", user.username)

		t.Logf("✓ User %s connected and subscribed successfully", user.username)
	}

	// Have the first user publish a message
	testMessage := "Config multi-user test message"
	pubToken := clients[0].Publish("config/multiuser", 1, false, testMessage)
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

	t.Log("Configuration-based multi-user test completed successfully")
}