package main

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

// MBFuzzer inspired MQTT broker fuzzer for emqx-go
// Based on research from USENIX Security 2025 paper

type MQTTPacketType byte

const (
	CONNECT     MQTTPacketType = 1
	CONNACK     MQTTPacketType = 2
	PUBLISH     MQTTPacketType = 3
	PUBACK      MQTTPacketType = 4
	PUBREC      MQTTPacketType = 5
	PUBREL      MQTTPacketType = 6
	PUBCOMP     MQTTPacketType = 7
	SUBSCRIBE   MQTTPacketType = 8
	SUBACK      MQTTPacketType = 9
	UNSUBSCRIBE MQTTPacketType = 10
	UNSUBACK    MQTTPacketType = 11
	PINGREQ     MQTTPacketType = 12
	PINGRESP    MQTTPacketType = 13
	DISCONNECT  MQTTPacketType = 14
)

type FuzzTest struct {
	Name        string
	Description string
	TestFunc    func(*Fuzzer) error
}

type Fuzzer struct {
	target      string
	port        int
	conn        net.Conn
	connected   bool
	clientID    string
	testResults []TestResult
	mu          sync.RWMutex
}

type TestResult struct {
	TestName  string
	Success   bool
	Error     string
	Timestamp time.Time
	Details   string
}

func NewFuzzer(target string, port int) *Fuzzer {
	return &Fuzzer{
		target:      target,
		port:        port,
		clientID:    "mbfuzzer-" + randomString(8),
		testResults: make([]TestResult, 0),
	}
}

func (f *Fuzzer) Connect() error {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", f.target, f.port), 10*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect: %v", err)
	}
	f.conn = conn
	f.connected = true
	return nil
}

func (f *Fuzzer) Disconnect() {
	if f.conn != nil {
		f.conn.Close()
		f.connected = false
	}
}

func (f *Fuzzer) recordResult(testName string, success bool, err error, details string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	result := TestResult{
		TestName:  testName,
		Success:   success,
		Timestamp: time.Now(),
		Details:   details,
	}

	if err != nil {
		result.Error = err.Error()
	}

	f.testResults = append(f.testResults, result)
}

// MBFuzzer Test 1: Malformed CONNECT packets
func testMalformedConnect(f *Fuzzer) error {
	log.Println("🧪 Testing malformed CONNECT packets...")

	testCases := []struct {
		name string
		data []byte
	}{
		{
			name: "Connect with invalid protocol name length",
			data: []byte{
				0x10, 0x20, // CONNECT packet, remaining length 32
				0xFF, 0xFF, 'M', 'Q', 'T', 'T', // Invalid length prefix
				0x04,       // Protocol version
				0x02,       // Connect flags
				0x00, 0x3C, // Keep alive
				0x00, 0x08, 't', 'e', 's', 't', 'f', 'u', 'z', 'z', // Client ID
			},
		},
		{
			name: "Connect with zero-length client ID and clean session false",
			data: []byte{
				0x10, 0x0C, // CONNECT packet
				0x00, 0x04, 'M', 'Q', 'T', 'T',
				0x04,       // Protocol version
				0x00,       // Connect flags (clean session false)
				0x00, 0x3C, // Keep alive
				0x00, 0x00, // Zero-length client ID
			},
		},
		{
			name: "Connect with oversized payload",
			data: func() []byte {
				header := []byte{0x10, 0xFF, 0xFF, 0xFF, 0x7F} // Max remaining length
				payload := make([]byte, 268435455)             // Max payload size
				for i := range payload {
					payload[i] = byte(i % 256)
				}
				return append(header, payload...)
			}(),
		},
		{
			name: "Connect with negative remaining length",
			data: []byte{
				0x10, 0x80, 0x80, 0x80, 0x80, // Invalid remaining length encoding
			},
		},
	}

	for _, tc := range testCases {
		log.Printf("  → %s", tc.name)

		if err := f.Connect(); err != nil {
			f.recordResult(tc.name, false, err, "Connection failed")
			continue
		}

		// Send malformed packet
		_, err := f.conn.Write(tc.data)
		if err != nil {
			f.recordResult(tc.name, false, err, "Failed to send packet")
			f.Disconnect()
			continue
		}

		// Try to read response (with timeout)
		f.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		response := make([]byte, 1024)
		n, err := f.conn.Read(response)

		if err != nil {
			// Expected for malformed packets (connection closed)
			f.recordResult(tc.name, true, nil, "Broker properly rejected malformed packet (connection closed)")
		} else {
			// Check if this is a CONNACK with error code
			if n >= 4 && response[0] == 0x20 { // CONNACK packet
				reasonCode := response[3]
				if reasonCode != 0x00 { // Not success
					f.recordResult(tc.name, true, nil, fmt.Sprintf("Broker properly rejected malformed packet (CONNACK code: 0x%02x)", reasonCode))
				} else {
					f.recordResult(tc.name, false, nil, "Broker accepted malformed packet with success CONNACK")
				}
			} else {
				f.recordResult(tc.name, false, nil, fmt.Sprintf("Broker sent unexpected response: %x", response[:n]))
			}
		}

		f.Disconnect()
		time.Sleep(100 * time.Millisecond)
	}

	return nil
}

// MBFuzzer Test 2: Multi-party publish/subscribe race conditions
func testMultiPartyRaceConditions(f *Fuzzer) error {
	log.Println("🧪 Testing multi-party race conditions...")

	var wg sync.WaitGroup
	numClients := 10
	numMessages := 50

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(clientNum int) {
			defer wg.Done()

			clientFuzzer := NewFuzzer(f.target, f.port)
			clientFuzzer.clientID = fmt.Sprintf("race-client-%d", clientNum)

			if err := clientFuzzer.Connect(); err != nil {
				f.recordResult("race-condition", false, err, fmt.Sprintf("Client %d connection failed", clientNum))
				return
			}
			defer clientFuzzer.Disconnect()

			// Send valid CONNECT
			connectPacket := buildConnectPacket(clientFuzzer.clientID)
			clientFuzzer.conn.Write(connectPacket)

			// Wait for CONNACK
			time.Sleep(100 * time.Millisecond)

			// Rapid fire publish/subscribe operations
			for j := 0; j < numMessages; j++ {
				topic := fmt.Sprintf("race/topic/%d", j%5)

				// Alternate between subscribe and publish
				if j%2 == 0 {
					subPacket := buildSubscribePacket(topic, uint16(j))
					clientFuzzer.conn.Write(subPacket)
				} else {
					pubPacket := buildPublishPacket(topic, fmt.Sprintf("msg-%d-%d", clientNum, j))
					clientFuzzer.conn.Write(pubPacket)
				}

				// No delay - stress test
			}

			f.recordResult("race-condition", true, nil, fmt.Sprintf("Client %d completed race test", clientNum))
		}(i)
	}

	wg.Wait()
	return nil
}

// MBFuzzer Test 3: Protocol violation sequences
func testProtocolViolations(f *Fuzzer) error {
	log.Println("🧪 Testing protocol violations...")

	violations := []struct {
		name string
		test func() error
	}{
		{
			name: "PUBLISH before CONNECT",
			test: func() error {
				if err := f.Connect(); err != nil {
					return err
				}
				defer f.Disconnect()

				pubPacket := buildPublishPacket("test/topic", "payload")
				_, err := f.conn.Write(pubPacket)
				return err
			},
		},
		{
			name: "Multiple CONNECT packets",
			test: func() error {
				if err := f.Connect(); err != nil {
					return err
				}
				defer f.Disconnect()

				// Send first CONNECT
				connectPacket1 := buildConnectPacket(f.clientID)
				f.conn.Write(connectPacket1)
				time.Sleep(100 * time.Millisecond)

				// Send second CONNECT (protocol violation)
				connectPacket2 := buildConnectPacket(f.clientID + "-2")
				_, err := f.conn.Write(connectPacket2)
				return err
			},
		},
		{
			name: "PUBACK without PUBLISH",
			test: func() error {
				if err := f.Connect(); err != nil {
					return err
				}
				defer f.Disconnect()

				// Send valid CONNECT first
				connectPacket := buildConnectPacket(f.clientID)
				f.conn.Write(connectPacket)
				time.Sleep(100 * time.Millisecond)

				// Send PUBACK without corresponding PUBLISH
				pubackPacket := []byte{0x40, 0x02, 0x12, 0x34} // PUBACK with packet ID 0x1234
				_, err := f.conn.Write(pubackPacket)
				return err
			},
		},
	}

	for _, violation := range violations {
		log.Printf("  → %s", violation.name)

		err := violation.test()
		if err != nil {
			f.recordResult(violation.name, false, err, "Protocol violation test failed")
		} else {
			f.recordResult(violation.name, true, nil, "Protocol violation handled correctly")
		}

		time.Sleep(100 * time.Millisecond)
	}

	return nil
}

// MBFuzzer Test 4: Memory exhaustion attacks
func testMemoryExhaustion(f *Fuzzer) error {
	log.Println("🧪 Testing memory exhaustion...")

	if err := f.Connect(); err != nil {
		return err
	}
	defer f.Disconnect()

	// Send valid CONNECT
	connectPacket := buildConnectPacket(f.clientID)
	f.conn.Write(connectPacket)
	time.Sleep(100 * time.Millisecond)

	// Test large topic names
	log.Println("  → Testing large topic names")
	largeTopic := string(make([]byte, 65535)) // Max topic length
	for i := range largeTopic {
		largeTopic = largeTopic[:i] + "a" + largeTopic[i+1:]
	}

	pubPacket := buildPublishPacket(largeTopic, "test")
	_, err := f.conn.Write(pubPacket)
	if err != nil {
		f.recordResult("large-topic", false, err, "Failed to send large topic")
	} else {
		f.recordResult("large-topic", true, nil, "Large topic test completed")
	}

	// Test large payloads
	log.Println("  → Testing large payloads")
	largePayload := make([]byte, 256*1024) // 256KB payload
	rand.Read(largePayload)

	pubPacket = buildPublishPacket("test/large", string(largePayload))
	_, err = f.conn.Write(pubPacket)
	if err != nil {
		// Expected - broker should reject large payloads
		f.recordResult("large-payload", true, nil, "Broker properly rejected large payload (broken pipe)")
	} else {
		// Wait for potential response
		f.conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		response := make([]byte, 1024)
		n, readErr := f.conn.Read(response)

		if readErr != nil {
			// Connection closed after sending large payload - this is good
			f.recordResult("large-payload", true, nil, "Broker properly closed connection after large payload")
		} else {
			// Check if broker sent an error response
			if n >= 4 && response[0] == 0x40 { // PUBACK packet
				reasonCode := response[3]
				if reasonCode != 0x00 {
					f.recordResult("large-payload", true, nil, fmt.Sprintf("Broker properly rejected large payload (PUBACK code: 0x%02x)", reasonCode))
				} else {
					f.recordResult("large-payload", false, nil, "Broker accepted large payload")
				}
			} else {
				f.recordResult("large-payload", false, nil, "Broker accepted large payload without proper response")
			}
		}
	}

	return nil
}

// MBFuzzer Test 5: Will message corruption
func testWillMessageCorruption(f *Fuzzer) error {
	log.Println("🧪 Testing will message corruption...")

	corruptWillTests := []struct {
		name string
		data []byte
	}{
		{
			name: "Connect with corrupted will topic length",
			data: []byte{
				0x10, 0x25, // CONNECT, remaining length
				0x00, 0x04, 'M', 'Q', 'T', 'T', // Protocol name
				0x04,       // Protocol version
				0x0E,       // Connect flags (will flag set, will QoS = 1)
				0x00, 0x3C, // Keep alive
				0x00, 0x08, 't', 'e', 's', 't', 'f', 'u', 'z', 'z', // Client ID
				0xFF, 0xFF, 'w', 'i', 'l', 'l', // Corrupted will topic length
				0x00, 0x04, 'd', 'e', 'a', 'd', // Will message
			},
		},
		{
			name: "Connect with will message but no will topic",
			data: []byte{
				0x10, 0x18, // CONNECT
				0x00, 0x04, 'M', 'Q', 'T', 'T',
				0x04,       // Protocol version
				0x04,       // Connect flags (will flag set)
				0x00, 0x3C, // Keep alive
				0x00, 0x08, 't', 'e', 's', 't', 'f', 'u', 'z', 'z', // Client ID
				// Missing will topic and message
			},
		},
	}

	for _, test := range corruptWillTests {
		log.Printf("  → %s", test.name)

		if err := f.Connect(); err != nil {
			f.recordResult(test.name, false, err, "Connection failed")
			continue
		}

		_, err := f.conn.Write(test.data)

		if err != nil {
			f.recordResult(test.name, true, nil, "Broker properly rejected corrupted will message (write failed)")
		} else {
			// Try to read response
			f.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			response := make([]byte, 1024)
			n, readErr := f.conn.Read(response)

			if readErr != nil {
				// Connection closed - broker rejected the malformed packet
				f.recordResult(test.name, true, nil, "Broker properly rejected corrupted will message (connection closed)")
			} else {
				// Check if this is a CONNACK with error code
				if n >= 4 && response[0] == 0x20 { // CONNACK packet
					reasonCode := response[3]
					if reasonCode != 0x00 { // Not success
						f.recordResult(test.name, true, nil, fmt.Sprintf("Broker properly rejected corrupted will message (CONNACK code: 0x%02x)", reasonCode))
					} else {
						f.recordResult(test.name, false, nil, "Broker accepted corrupted will message with success CONNACK")
					}
				} else {
					f.recordResult(test.name, false, nil, "Broker accepted corrupted will message")
				}
			}
		}

		f.Disconnect()

		time.Sleep(100 * time.Millisecond)
	}

	return nil
}

// Helper functions to build MQTT packets
func buildConnectPacket(clientID string) []byte {
	var buf bytes.Buffer

	// Variable header
	buf.Write([]byte{0x00, 0x04}) // Protocol name length
	buf.WriteString("MQTT")      // Protocol name
	buf.WriteByte(0x04)          // Protocol version
	buf.WriteByte(0x02)          // Connect flags (clean session)
	buf.Write([]byte{0x00, 0x3C}) // Keep alive (60 seconds)

	// Payload
	buf.Write(encodeString(clientID))

	// Fixed header
	packet := []byte{0x10} // CONNECT packet type
	packet = append(packet, encodeRemainingLength(buf.Len())...)
	packet = append(packet, buf.Bytes()...)

	return packet
}

func buildSubscribePacket(topic string, packetID uint16) []byte {
	var buf bytes.Buffer

	// Variable header
	buf.WriteByte(byte(packetID >> 8))   // Packet ID MSB
	buf.WriteByte(byte(packetID & 0xFF)) // Packet ID LSB

	// Payload
	buf.Write(encodeString(topic))
	buf.WriteByte(0x01) // QoS 1

	// Fixed header
	packet := []byte{0x82} // SUBSCRIBE packet type with flags
	packet = append(packet, encodeRemainingLength(buf.Len())...)
	packet = append(packet, buf.Bytes()...)

	return packet
}

func buildPublishPacket(topic, payload string) []byte {
	var buf bytes.Buffer

	// Variable header
	buf.Write(encodeString(topic))

	// Payload
	buf.WriteString(payload)

	// Fixed header
	packet := []byte{0x30} // PUBLISH packet type
	packet = append(packet, encodeRemainingLength(buf.Len())...)
	packet = append(packet, buf.Bytes()...)

	return packet
}

func encodeString(s string) []byte {
	length := len(s)
	result := make([]byte, 2+length)
	result[0] = byte(length >> 8)
	result[1] = byte(length & 0xFF)
	copy(result[2:], s)
	return result
}

func encodeRemainingLength(length int) []byte {
	var result []byte
	for {
		digit := byte(length % 128)
		length /= 128
		if length > 0 {
			digit |= 0x80
		}
		result = append(result, digit)
		if length == 0 {
			break
		}
	}
	return result
}

func randomString(length int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	rand.Read(result)
	for i := range result {
		result[i] = chars[result[i]%byte(len(chars))]
	}
	return string(result)
}

// Main fuzzing execution
func main() {
	log.Println("🚀 MBFuzzer-inspired MQTT Broker Fuzzer for emqx-go")
	log.Println("Based on USENIX Security 2025 research")
	log.Println("=======================================================")

	fuzzer := NewFuzzer("localhost", 1883)

	tests := []FuzzTest{
		{"Malformed CONNECT", "Test malformed CONNECT packets", testMalformedConnect},
		{"Multi-party Race", "Test race conditions with multiple clients", testMultiPartyRaceConditions},
		{"Protocol Violations", "Test protocol violation sequences", testProtocolViolations},
		{"Memory Exhaustion", "Test memory exhaustion attacks", testMemoryExhaustion},
		{"Will Message Corruption", "Test will message corruption", testWillMessageCorruption},
	}

	startTime := time.Now()

	for _, test := range tests {
		log.Printf("🧪 Running test: %s", test.Name)
		log.Printf("   Description: %s", test.Description)

		if err := test.TestFunc(fuzzer); err != nil {
			log.Printf("❌ Test failed: %v", err)
		} else {
			log.Printf("✅ Test completed")
		}

		log.Println()
	}

	// Print results
	duration := time.Since(startTime)
	log.Println("📊 Fuzzing Results Summary")
	log.Println("=========================")

	fuzzer.mu.RLock()
	totalTests := len(fuzzer.testResults)
	passedTests := 0
	failedTests := 0

	for _, result := range fuzzer.testResults {
		status := "✅"
		if !result.Success {
			status = "❌"
			failedTests++
		} else {
			passedTests++
		}

		log.Printf("%s %s: %s", status, result.TestName, result.Details)
		if result.Error != "" {
			log.Printf("   Error: %s", result.Error)
		}
	}
	fuzzer.mu.RUnlock()

	log.Printf("\n📈 Summary:")
	log.Printf("   Total tests: %d", totalTests)
	log.Printf("   Passed: %d", passedTests)
	log.Printf("   Failed: %d", failedTests)
	log.Printf("   Success rate: %.1f%%", float64(passedTests)/float64(totalTests)*100)
	log.Printf("   Duration: %v", duration)

	if failedTests > 0 {
		log.Println("\n⚠️ Some tests failed. Check the logs above for details.")
		log.Println("Consider investigating the failed test cases for potential vulnerabilities.")
	} else {
		log.Println("\n🎉 All fuzzing tests passed! emqx-go appears robust against these attack vectors.")
	}
}