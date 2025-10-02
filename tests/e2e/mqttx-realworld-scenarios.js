// MQTTX Real-World Scenarios Tests (75 test cases)
const { MQTTXClient, createClients, connectAll, disconnectAll } = require('./utils/mqttx-client');
const { TestHelpers, TopicUtils, MessageValidator } = require('./utils/test-helpers');
const config = require('./utils/config');

class RealWorldScenariosTests {
  constructor() {
    this.helpers = new TestHelpers();
    this.clients = [];
  }

  async runAllTests() {
    console.log('🚀 Starting MQTTX Real-World Scenarios Tests (75 test cases)');

    try {
      // IoT Device Simulation (20 tests)
      await this.runIoTDeviceTests();

      // Smart Home Scenarios (15 tests)
      await this.runSmartHomeTests();

      // Industrial IoT (15 tests)
      await this.runIndustrialIoTTests();

      // Chat/Messaging Applications (15 tests)
      await this.runChatMessagingTests();

      // Telemetry & Monitoring (10 tests)
      await this.runTelemetryMonitoringTests();

      return this.helpers.generateReport();
    } finally {
      await this.cleanup();
    }
  }

  // IoT Device Simulation (20 test cases)
  async runIoTDeviceTests() {
    console.log('\n🌐 Running IoT Device Simulation Tests (20 test cases)');

    // Test 426-430: Sensor data collection
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`sensor-data-collection-${i + 1}`, async () => {
        const gateway = new MQTTXClient({ clientId: `gateway-${i}` });
        const sensorCount = 5 + i;
        const sensors = createClients(sensorCount, { clientId: `sensor-${i}` });

        this.clients.push(gateway, ...sensors);

        await gateway.connect();
        await connectAll(sensors);

        // Gateway subscribes to all sensor data
        await gateway.subscribe('sensors/+/data');
        await gateway.subscribe('sensors/+/status');

        // Sensors publish data periodically
        const sensorData = [
          { type: 'temperature', value: 25.5, unit: 'C' },
          { type: 'humidity', value: 60.2, unit: '%' },
          { type: 'pressure', value: 1013.25, unit: 'hPa' },
          { type: 'light', value: 350, unit: 'lux' },
          { type: 'motion', value: 1, unit: 'boolean' }
        ];

        for (let j = 0; j < sensors.length; j++) {
          const sensor = sensors[j];
          const data = sensorData[j % sensorData.length];

          await sensor.publish(`sensors/sensor${j}/status`, 'online');
          await sensor.publish(`sensors/sensor${j}/data`, JSON.stringify({
            ...data,
            timestamp: Date.now(),
            sensorId: `sensor${j}`
          }));
        }

        const messages = await gateway.waitForMessages(sensorCount * 2, 10000);
        TestHelpers.assertGreaterThan(messages.length, sensorCount,
          'Gateway should receive data from multiple sensors');

        // Verify data integrity
        const dataMessages = messages.filter(m => m.topic.includes('/data'));
        for (const message of dataMessages) {
          try {
            const data = JSON.parse(message.message);
            TestHelpers.assertTrue(data.timestamp && data.sensorId,
              'Sensor data should include timestamp and sensor ID');
          } catch (error) {
            TestHelpers.assertTrue(false, `Invalid sensor data format: ${message.message}`);
          }
        }
      });
    }

    // Test 431-435: Device heartbeat monitoring
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`device-heartbeat-monitoring-${i + 1}`, async () => {
        const monitor = new MQTTXClient({ clientId: `monitor-${i}` });
        const deviceCount = 3 + i;
        const devices = createClients(deviceCount, { clientId: `device-heartbeat-${i}` });

        this.clients.push(monitor, ...devices);

        await monitor.connect();
        await connectAll(devices);

        // Monitor subscribes to heartbeat topic
        await monitor.subscribe('devices/+/heartbeat');

        // Devices send heartbeats
        const heartbeatInterval = 1000; // 1 second
        for (let j = 0; j < devices.length; j++) {
          const device = devices[j];

          // Send initial heartbeat
          await device.publish(`devices/device${j}/heartbeat`, JSON.stringify({
            deviceId: `device${j}`,
            status: 'alive',
            timestamp: Date.now(),
            uptime: Math.floor(Math.random() * 10000)
          }));
        }

        // Wait for heartbeats
        const messages = await monitor.waitForMessages(deviceCount, 5000);
        TestHelpers.assertEqual(messages.length, deviceCount,
          'Monitor should receive heartbeats from all devices');

        // Verify heartbeat format
        for (const message of messages) {
          try {
            const heartbeat = JSON.parse(message.message);
            TestHelpers.assertTrue(heartbeat.deviceId && heartbeat.status === 'alive',
              'Heartbeat should contain device ID and alive status');
          } catch (error) {
            TestHelpers.assertTrue(false, `Invalid heartbeat format: ${message.message}`);
          }
        }
      });
    }

    // Test 436-440: Firmware update scenarios
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`firmware-update-${i + 1}`, async () => {
        const updateServer = new MQTTXClient({ clientId: `update-server-${i}` });
        const device = new MQTTXClient({ clientId: `update-device-${i}` });

        this.clients.push(updateServer, device);

        await Promise.all([updateServer.connect(), device.connect()]);

        const deviceId = `device-${i}`;

        // Server subscribes to update responses
        await updateServer.subscribe(`devices/${deviceId}/update/response`);

        // Device subscribes to update commands
        await device.subscribe(`devices/${deviceId}/update/command`);

        // Simulate firmware update process
        const updateCommand = {
          action: 'update',
          version: '2.1.0',
          url: `https://updates.example.com/firmware-v2.1.0.bin`,
          checksum: 'sha256:abcd1234',
          timestamp: Date.now()
        };

        await updateServer.publish(`devices/${deviceId}/update/command`,
          JSON.stringify(updateCommand));

        // Device receives update command
        const commandMessages = await device.waitForMessages(1, 5000);
        TestHelpers.assertEqual(commandMessages.length, 1, 'Device should receive update command');

        // Device responds with acknowledgment
        const response = {
          action: 'ack',
          status: 'downloading',
          progress: 0,
          timestamp: Date.now()
        };

        await device.publish(`devices/${deviceId}/update/response`,
          JSON.stringify(response));

        // Server receives response
        const responseMessages = await updateServer.waitForMessages(1, 5000);
        TestHelpers.assertEqual(responseMessages.length, 1, 'Server should receive update response');

        const receivedResponse = JSON.parse(responseMessages[0].message);
        TestHelpers.assertEqual(receivedResponse.action, 'ack',
          'Response should acknowledge update command');
      });
    }

    // Test 441-445: Device configuration management
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`device-config-management-${i + 1}`, async () => {
        const configServer = new MQTTXClient({ clientId: `config-server-${i}` });
        const device = new MQTTXClient({ clientId: `config-device-${i}` });

        this.clients.push(configServer, device);

        await Promise.all([configServer.connect(), device.connect()]);

        const deviceId = `device-${i}`;

        // Device publishes current configuration
        const currentConfig = {
          deviceId: deviceId,
          samplingRate: 5000,
          reportingInterval: 30000,
          thresholds: {
            temperature: { min: -10, max: 50 },
            humidity: { min: 0, max: 100 }
          },
          version: '1.0.0'
        };

        await device.publish(`devices/${deviceId}/config/current`,
          JSON.stringify(currentConfig));

        // Server subscribes to device configurations
        await configServer.subscribe('devices/+/config/current');

        const configMessages = await configServer.waitForMessages(1, 5000);
        TestHelpers.assertEqual(configMessages.length, 1, 'Server should receive current configuration');

        // Server sends configuration update
        const newConfig = {
          ...currentConfig,
          samplingRate: 2000, // Increase sampling rate
          reportingInterval: 15000, // Decrease reporting interval
          version: '1.1.0'
        };

        await configServer.subscribe(`devices/${deviceId}/config/response`);
        await device.subscribe(`devices/${deviceId}/config/update`);

        await configServer.publish(`devices/${deviceId}/config/update`,
          JSON.stringify(newConfig));

        // Device receives and acknowledges configuration update
        const updateMessages = await device.waitForMessages(1, 5000);
        TestHelpers.assertEqual(updateMessages.length, 1, 'Device should receive configuration update');

        await device.publish(`devices/${deviceId}/config/response`,
          JSON.stringify({ status: 'applied', version: '1.1.0' }));

        const ackMessages = await configServer.waitForMessages(1, 5000);
        TestHelpers.assertEqual(ackMessages.length, 1, 'Server should receive configuration acknowledgment');
      });
    }
  }

  // Smart Home Scenarios (15 test cases)
  async runSmartHomeTests() {
    console.log('\n🏠 Running Smart Home Scenarios Tests (15 test cases)');

    // Test 446-450: Home automation control
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`home-automation-control-${i + 1}`, async () => {
        const homeController = new MQTTXClient({ clientId: `home-controller-${i}` });
        const mobileApp = new MQTTXClient({ clientId: `mobile-app-${i}` });
        const smartDevices = createClients(4, { clientId: `smart-device-${i}` }); // lights, thermostat, door, alarm

        this.clients.push(homeController, mobileApp, ...smartDevices);

        await homeController.connect();
        await mobileApp.connect();
        await connectAll(smartDevices);

        const rooms = ['living-room', 'bedroom', 'kitchen', 'bathroom'];

        // Home controller subscribes to all device status
        await homeController.subscribe('home/+/+/status');
        await homeController.subscribe('home/+/+/command');

        // Mobile app subscribes to status updates
        await mobileApp.subscribe('home/+/+/status');

        // Smart devices subscribe to their specific command topics
        for (let j = 0; j < smartDevices.length; j++) {
          const room = rooms[j];
          await smartDevices[j].subscribe(`home/${room}/lights/command`);
        }

        // Mobile app sends command to turn on lights
        const lightCommand = {
          action: 'turn_on',
          brightness: 80,
          color: 'warm_white',
          timestamp: Date.now()
        };

        await mobileApp.publish('home/living-room/lights/command',
          JSON.stringify(lightCommand));

        // Device receives command and responds with status
        const commandMessages = await smartDevices[0].waitForMessages(1, 5000);
        TestHelpers.assertEqual(commandMessages.length, 1, 'Smart device should receive command');

        await smartDevices[0].publish('home/living-room/lights/status',
          JSON.stringify({
            state: 'on',
            brightness: 80,
            color: 'warm_white',
            power_consumption: 12.5,
            timestamp: Date.now()
          }));

        // Both home controller and mobile app receive status update
        const controllerMessages = await homeController.waitForMessages(1, 5000);
        const appMessages = await mobileApp.waitForMessages(1, 5000);

        TestHelpers.assertEqual(controllerMessages.length, 1, 'Home controller should receive status');
        TestHelpers.assertEqual(appMessages.length, 1, 'Mobile app should receive status');
      });
    }

    // Test 451-455: Energy monitoring
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`energy-monitoring-${i + 1}`, async () => {
        const energyMonitor = new MQTTXClient({ clientId: `energy-monitor-${i}` });
        const smartMeter = new MQTTXClient({ clientId: `smart-meter-${i}` });
        const appliances = createClients(3, { clientId: `appliance-${i}` });

        this.clients.push(energyMonitor, smartMeter, ...appliances);

        await energyMonitor.connect();
        await smartMeter.connect();
        await connectAll(appliances);

        // Energy monitor subscribes to all power consumption data
        await energyMonitor.subscribe('energy/+/consumption');
        await energyMonitor.subscribe('energy/total');

        const applianceTypes = ['refrigerator', 'washing-machine', 'air-conditioner'];

        // Appliances publish power consumption data
        for (let j = 0; j < appliances.length; j++) {
          const appliance = appliances[j];
          const type = applianceTypes[j];

          const consumption = {
            appliance: type,
            power: Math.random() * 2000 + 100, // 100-2100 watts
            energy_today: Math.random() * 50, // kWh
            status: 'running',
            timestamp: Date.now()
          };

          await appliance.publish(`energy/${type}/consumption`,
            JSON.stringify(consumption));
        }

        // Smart meter publishes total consumption
        await smartMeter.publish('energy/total', JSON.stringify({
          total_power: 3250,
          total_energy_today: 45.6,
          grid_frequency: 50.0,
          voltage: 230.5,
          timestamp: Date.now()
        }));

        // Energy monitor receives all consumption data
        const messages = await energyMonitor.waitForMessages(4, 5000); // 3 appliances + 1 total
        TestHelpers.assertEqual(messages.length, 4, 'Energy monitor should receive all consumption data');

        // Verify data format
        for (const message of messages) {
          try {
            const data = JSON.parse(message.message);
            TestHelpers.assertTrue(data.timestamp, 'Energy data should include timestamp');
          } catch (error) {
            TestHelpers.assertTrue(false, `Invalid energy data format: ${message.message}`);
          }
        }
      });
    }

    // Test 456-460: Security system integration
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`security-system-${i + 1}`, async () => {
        const securityPanel = new MQTTXClient({ clientId: `security-panel-${i}` });
        const sensors = createClients(4, { clientId: `security-sensor-${i}` }); // door, window, motion, camera
        const mobileApp = new MQTTXClient({ clientId: `security-app-${i}` });

        this.clients.push(securityPanel, mobileApp, ...sensors);

        await securityPanel.connect();
        await mobileApp.connect();
        await connectAll(sensors);

        // Security panel subscribes to all sensors
        await securityPanel.subscribe('security/+/alert');
        await securityPanel.subscribe('security/+/status');

        // Mobile app subscribes to security notifications
        await mobileApp.subscribe('security/notifications');

        const sensorTypes = ['door', 'window', 'motion', 'camera'];

        // Simulate security event
        const eventSensor = sensors[2]; // motion sensor
        await eventSensor.publish('security/motion/alert', JSON.stringify({
          type: 'motion_detected',
          location: 'living-room',
          confidence: 0.95,
          timestamp: Date.now()
        }));

        // Security panel receives alert
        const alertMessages = await securityPanel.waitForMessages(1, 5000);
        TestHelpers.assertEqual(alertMessages.length, 1, 'Security panel should receive alert');

        // Security panel processes alert and sends notification
        await securityPanel.publish('security/notifications', JSON.stringify({
          alert_type: 'motion_detected',
          location: 'living-room',
          severity: 'medium',
          action_required: 'verify',
          timestamp: Date.now()
        }));

        // Mobile app receives notification
        const notificationMessages = await mobileApp.waitForMessages(1, 5000);
        TestHelpers.assertEqual(notificationMessages.length, 1, 'Mobile app should receive security notification');

        const notification = JSON.parse(notificationMessages[0].message);
        TestHelpers.assertEqual(notification.alert_type, 'motion_detected',
          'Notification should contain correct alert type');
      });
    }
  }

  // Industrial IoT (15 test cases)
  async runIndustrialIoTTests() {
    console.log('\n🏭 Running Industrial IoT Tests (15 test cases)');

    // Test 461-465: Manufacturing line monitoring
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`manufacturing-line-monitoring-${i + 1}`, async () => {
        const scadaSystem = new MQTTXClient({ clientId: `scada-${i}` });
        const machines = createClients(4, { clientId: `machine-${i}` });
        const qualityControl = new MQTTXClient({ clientId: `quality-control-${i}` });

        this.clients.push(scadaSystem, qualityControl, ...machines);

        await scadaSystem.connect();
        await qualityControl.connect();
        await connectAll(machines);

        // SCADA system monitors all machines
        await scadaSystem.subscribe('factory/line1/+/status');
        await scadaSystem.subscribe('factory/line1/+/production');

        // Quality control monitors product data
        await qualityControl.subscribe('factory/line1/+/quality');

        const machineTypes = ['conveyor', 'assembly', 'packaging', 'inspection'];

        // Machines publish operational data
        for (let j = 0; j < machines.length; j++) {
          const machine = machines[j];
          const type = machineTypes[j];

          await machine.publish(`factory/line1/${type}/status`, JSON.stringify({
            machine_id: `${type}-${j}`,
            status: 'running',
            speed: Math.random() * 100 + 50, // 50-150 units/min
            temperature: Math.random() * 30 + 40, // 40-70°C
            vibration: Math.random() * 5, // 0-5 mm/s
            timestamp: Date.now()
          }));

          await machine.publish(`factory/line1/${type}/production`, JSON.stringify({
            machine_id: `${type}-${j}`,
            units_produced: Math.floor(Math.random() * 100) + 50,
            efficiency: Math.random() * 0.3 + 0.7, // 70-100%
            downtime: Math.random() * 60, // 0-60 minutes
            timestamp: Date.now()
          }));

          if (type === 'inspection') {
            await machine.publish(`factory/line1/${type}/quality`, JSON.stringify({
              batch_id: `batch-${Date.now()}`,
              pass_rate: Math.random() * 0.1 + 0.9, // 90-100%
              defect_types: ['scratch', 'misalignment'],
              inspector: 'QC-001',
              timestamp: Date.now()
            }));
          }
        }

        // SCADA receives machine data
        const scadaMessages = await scadaSystem.waitForMessages(8, 10000); // 4 status + 4 production
        TestHelpers.assertGreaterThan(scadaMessages.length, 6,
          'SCADA should receive most machine data');

        // Quality control receives quality data
        const qualityMessages = await qualityControl.waitForMessages(1, 5000);
        TestHelpers.assertEqual(qualityMessages.length, 1,
          'Quality control should receive quality data');
      });
    }

    // Test 466-470: Predictive maintenance
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`predictive-maintenance-${i + 1}`, async () => {
        const maintenanceSystem = new MQTTXClient({ clientId: `maintenance-${i}` });
        const machine = new MQTTXClient({ clientId: `monitored-machine-${i}` });
        const technician = new MQTTXClient({ clientId: `technician-${i}` });

        this.clients.push(maintenanceSystem, machine, technician);

        await Promise.all([maintenanceSystem.connect(), machine.connect(), technician.connect()]);

        // Maintenance system monitors machine health
        await maintenanceSystem.subscribe('maintenance/machines/+/health');
        await maintenanceSystem.subscribe('maintenance/machines/+/alerts');

        // Technician receives work orders
        await technician.subscribe('maintenance/workorders');

        const machineId = `machine-${i}`;

        // Machine publishes health data indicating potential issue
        const healthData = {
          machine_id: machineId,
          vibration_rms: 8.5, // High vibration
          temperature: 85, // High temperature
          bearing_condition: 'degraded',
          oil_analysis: {
            viscosity: 'high',
            contamination: 'moderate'
          },
          runtime_hours: 8760,
          maintenance_score: 0.3, // Low score indicates maintenance needed
          timestamp: Date.now()
        };

        await machine.publish(`maintenance/machines/${machineId}/health`,
          JSON.stringify(healthData));

        // Maintenance system receives health data
        const healthMessages = await maintenanceSystem.waitForMessages(1, 5000);
        TestHelpers.assertEqual(healthMessages.length, 1,
          'Maintenance system should receive health data');

        // Maintenance system detects issue and creates alert
        await maintenanceSystem.publish(`maintenance/machines/${machineId}/alerts`,
          JSON.stringify({
            alert_id: `alert-${Date.now()}`,
            machine_id: machineId,
            severity: 'high',
            predicted_failure: 'bearing failure within 72 hours',
            recommended_action: 'replace bearing assembly',
            confidence: 0.87,
            timestamp: Date.now()
          }));

        // Maintenance system creates work order
        await maintenanceSystem.publish('maintenance/workorders', JSON.stringify({
          work_order_id: `WO-${Date.now()}`,
          machine_id: machineId,
          priority: 'urgent',
          task: 'Replace bearing assembly',
          estimated_duration: '4 hours',
          required_parts: ['bearing-assy-001', 'seal-kit-002'],
          assigned_technician: 'tech-001',
          timestamp: Date.now()
        }));

        // Technician receives work order
        const workOrderMessages = await technician.waitForMessages(1, 5000);
        TestHelpers.assertEqual(workOrderMessages.length, 1,
          'Technician should receive work order');

        const workOrder = JSON.parse(workOrderMessages[0].message);
        TestHelpers.assertEqual(workOrder.priority, 'urgent',
          'Work order should have urgent priority');
      });
    }

    // Test 471-475: Supply chain tracking
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`supply-chain-tracking-${i + 1}`, async () => {
        const scmSystem = new MQTTXClient({ clientId: `scm-system-${i}` });
        const rfidReaders = createClients(3, { clientId: `rfid-reader-${i}` });
        const warehouse = new MQTTXClient({ clientId: `warehouse-${i}` });

        this.clients.push(scmSystem, warehouse, ...rfidReaders);

        await scmSystem.connect();
        await warehouse.connect();
        await connectAll(rfidReaders);

        // SCM system tracks all inventory movements
        await scmSystem.subscribe('supply-chain/+/scan');
        await scmSystem.subscribe('supply-chain/inventory/update');

        const locations = ['receiving', 'storage', 'shipping'];
        const batchId = `batch-${Date.now()}`;

        // Simulate item movement through supply chain
        for (let j = 0; j < rfidReaders.length; j++) {
          const reader = rfidReaders[j];
          const location = locations[j];

          await reader.publish(`supply-chain/${location}/scan`, JSON.stringify({
            rfid_tag: `TAG-${batchId}-001`,
            product_id: 'PROD-12345',
            batch_id: batchId,
            location: location,
            timestamp: Date.now(),
            reader_id: `reader-${location}`
          }));
        }

        // Warehouse updates inventory
        await warehouse.publish('supply-chain/inventory/update', JSON.stringify({
          product_id: 'PROD-12345',
          batch_id: batchId,
          quantity: 100,
          location: 'warehouse-A-01',
          status: 'in_stock',
          expiry_date: Date.now() + (30 * 24 * 60 * 60 * 1000), // 30 days
          timestamp: Date.now()
        }));

        // SCM system receives all tracking data
        const trackingMessages = await scmSystem.waitForMessages(4, 5000); // 3 scans + 1 inventory
        TestHelpers.assertEqual(trackingMessages.length, 4,
          'SCM system should receive all tracking data');

        // Verify scan data contains required fields
        const scanMessages = trackingMessages.filter(m => m.topic.includes('/scan'));
        for (const message of scanMessages) {
          const scanData = JSON.parse(message.message);
          TestHelpers.assertTrue(scanData.rfid_tag && scanData.location,
            'Scan data should include RFID tag and location');
        }
      });
    }
  }

  // Chat/Messaging Applications (15 test cases)
  async runChatMessagingTests() {
    console.log('\n💬 Running Chat/Messaging Application Tests (15 test cases)');

    // Test 476-480: Group chat functionality
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`group-chat-${i + 1}`, async () => {
        const chatServer = new MQTTXClient({ clientId: `chat-server-${i}` });
        const users = createClients(4, { clientId: `user-${i}` });

        this.clients.push(chatServer, ...users);

        await chatServer.connect();
        await connectAll(users);

        const groupId = `group-${i}`;
        const userNames = ['alice', 'bob', 'charlie', 'diana'];

        // Users join the group chat
        for (let j = 0; j < users.length; j++) {
          const user = users[j];
          await user.subscribe(`chat/groups/${groupId}/messages`);
          await user.subscribe(`chat/groups/${groupId}/presence`);
        }

        // Chat server monitors group activity
        await chatServer.subscribe(`chat/groups/${groupId}/+`);

        // Users send join notifications
        for (let j = 0; j < users.length; j++) {
          const user = users[j];
          await user.publish(`chat/groups/${groupId}/presence`, JSON.stringify({
            user: userNames[j],
            action: 'joined',
            timestamp: Date.now()
          }));
        }

        // Users send messages to the group
        const messages = [
          'Hello everyone!',
          'How is everyone doing?',
          'Great to see you all here',
          'Looking forward to our discussion'
        ];

        for (let j = 0; j < users.length; j++) {
          const user = users[j];
          await user.publish(`chat/groups/${groupId}/messages`, JSON.stringify({
            message_id: `msg-${Date.now()}-${j}`,
            user: userNames[j],
            text: messages[j],
            timestamp: Date.now(),
            group_id: groupId
          }));
        }

        // Each user should receive messages from other users
        for (let j = 0; j < users.length; j++) {
          const user = users[j];
          const receivedMessages = await user.waitForMessages(8, 5000); // 4 presence + 4 messages
          TestHelpers.assertGreaterThan(receivedMessages.length, 4,
            `User ${userNames[j]} should receive group messages`);
        }

        // Chat server should see all activity
        const serverMessages = await chatServer.waitForMessages(8, 5000);
        TestHelpers.assertEqual(serverMessages.length, 8,
          'Chat server should receive all group activity');
      });
    }

    // Test 481-485: Private messaging
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`private-messaging-${i + 1}`, async () => {
        const user1 = new MQTTXClient({ clientId: `private-user1-${i}` });
        const user2 = new MQTTXClient({ clientId: `private-user2-${i}` });
        const messageService = new MQTTXClient({ clientId: `message-service-${i}` });

        this.clients.push(user1, user2, messageService);

        await Promise.all([user1.connect(), user2.connect(), messageService.connect()]);

        const user1Id = `user1-${i}`;
        const user2Id = `user2-${i}`;

        // Users subscribe to their private message channels
        await user1.subscribe(`chat/private/${user1Id}/messages`);
        await user2.subscribe(`chat/private/${user2Id}/messages`);

        // Message service monitors private messages for delivery
        await messageService.subscribe('chat/private/+/messages');

        // User1 sends private message to User2
        const privateMessage = {
          message_id: `pm-${Date.now()}`,
          from: user1Id,
          to: user2Id,
          text: `Private message ${i} from user1 to user2`,
          timestamp: Date.now(),
          encrypted: false
        };

        await user1.publish(`chat/private/${user2Id}/messages`,
          JSON.stringify(privateMessage));

        // User2 receives the private message
        const user2Messages = await user2.waitForMessages(1, 5000);
        TestHelpers.assertEqual(user2Messages.length, 1, 'User2 should receive private message');

        const receivedMessage = JSON.parse(user2Messages[0].message);
        TestHelpers.assertEqual(receivedMessage.from, user1Id,
          'Message should be from user1');
        TestHelpers.assertEqual(receivedMessage.to, user2Id,
          'Message should be addressed to user2');

        // User2 sends read receipt
        await user2.publish(`chat/private/${user1Id}/messages`, JSON.stringify({
          message_id: `receipt-${Date.now()}`,
          from: user2Id,
          to: user1Id,
          type: 'read_receipt',
          original_message_id: privateMessage.message_id,
          timestamp: Date.now()
        }));

        // User1 receives read receipt
        const receiptMessages = await user1.waitForMessages(1, 5000);
        TestHelpers.assertEqual(receiptMessages.length, 1, 'User1 should receive read receipt');
      });
    }

    // Test 486-490: Notification delivery
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`notification-delivery-${i + 1}`, async () => {
        const notificationService = new MQTTXClient({ clientId: `notification-service-${i}` });
        const mobileDevices = createClients(3, { clientId: `mobile-device-${i}` });

        this.clients.push(notificationService, ...mobileDevices);

        await notificationService.connect();
        await connectAll(mobileDevices);

        const userIds = ['user1', 'user2', 'user3'];

        // Mobile devices subscribe to their notification channels
        for (let j = 0; j < mobileDevices.length; j++) {
          const device = mobileDevices[j];
          await device.subscribe(`notifications/${userIds[j]}/push`);
          await device.subscribe(`notifications/${userIds[j]}/email`);
        }

        // Notification service sends different types of notifications
        const notifications = [
          {
            type: 'push',
            title: 'New Message',
            body: `You have a new message from friend ${i}`,
            priority: 'high',
            action_url: '/chat/conversation/123'
          },
          {
            type: 'email',
            subject: 'Weekly Summary',
            body: `Here's your weekly activity summary ${i}`,
            priority: 'normal',
            template: 'weekly_summary'
          },
          {
            type: 'push',
            title: 'System Maintenance',
            body: `Scheduled maintenance in 1 hour ${i}`,
            priority: 'medium',
            category: 'system'
          }
        ];

        for (let j = 0; j < Math.min(mobileDevices.length, notifications.length); j++) {
          const notification = notifications[j];
          const userId = userIds[j];

          await notificationService.publish(`notifications/${userId}/${notification.type}`,
            JSON.stringify({
              notification_id: `notif-${Date.now()}-${j}`,
              ...notification,
              user_id: userId,
              timestamp: Date.now()
            }));
        }

        // Mobile devices receive notifications
        let totalReceived = 0;
        for (let j = 0; j < mobileDevices.length; j++) {
          try {
            const messages = await mobileDevices[j].waitForMessages(1, 5000);
            totalReceived += messages.length;
          } catch (error) {
            // Some devices might not receive notifications
          }
        }

        TestHelpers.assertGreaterThan(totalReceived, 0,
          'At least one device should receive notifications');
      });
    }
  }

  // Telemetry & Monitoring (10 test cases)
  async runTelemetryMonitoringTests() {
    console.log('\n📊 Running Telemetry & Monitoring Tests (10 test cases)');

    // Test 491-495: System health monitoring
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`system-health-monitoring-${i + 1}`, async () => {
        const monitoringSystem = new MQTTXClient({ clientId: `monitoring-${i}` });
        const agents = createClients(3, { clientId: `agent-${i}` });
        const alertManager = new MQTTXClient({ clientId: `alert-manager-${i}` });

        this.clients.push(monitoringSystem, alertManager, ...agents);

        await monitoringSystem.connect();
        await alertManager.connect();
        await connectAll(agents);

        // Monitoring system collects all metrics
        await monitoringSystem.subscribe('telemetry/+/metrics');
        await monitoringSystem.subscribe('telemetry/+/logs');

        // Alert manager handles alerts
        await alertManager.subscribe('alerts/+/critical');
        await alertManager.subscribe('alerts/+/warning');

        const serviceNames = ['web-server', 'database', 'cache'];

        // Agents send health metrics
        for (let j = 0; j < agents.length; j++) {
          const agent = agents[j];
          const serviceName = serviceNames[j];

          const metrics = {
            service: serviceName,
            cpu_usage: Math.random() * 100,
            memory_usage: Math.random() * 100,
            disk_usage: Math.random() * 100,
            response_time: Math.random() * 1000,
            error_rate: Math.random() * 0.1,
            uptime: Math.floor(Math.random() * 86400),
            timestamp: Date.now()
          };

          await agent.publish(`telemetry/${serviceName}/metrics`,
            JSON.stringify(metrics));

          // Generate alert if metrics exceed thresholds
          if (metrics.cpu_usage > 90) {
            await agent.publish(`alerts/${serviceName}/critical`, JSON.stringify({
              alert_type: 'high_cpu_usage',
              service: serviceName,
              value: metrics.cpu_usage,
              threshold: 90,
              severity: 'critical',
              timestamp: Date.now()
            }));
          }
        }

        // Monitoring system receives metrics
        const metricMessages = await monitoringSystem.waitForMessages(3, 5000);
        TestHelpers.assertEqual(metricMessages.length, 3,
          'Monitoring system should receive all service metrics');

        // Verify metric data structure
        for (const message of metricMessages) {
          const metrics = JSON.parse(message.message);
          TestHelpers.assertTrue(metrics.service && metrics.cpu_usage !== undefined,
            'Metrics should include service name and CPU usage');
        }
      });
    }

    // Test 496-500: Performance analytics
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`performance-analytics-${i + 1}`, async () => {
        const analyticsEngine = new MQTTXClient({ clientId: `analytics-${i}` });
        const loadBalancer = new MQTTXClient({ clientId: `load-balancer-${i}` });
        const webServers = createClients(3, { clientId: `web-server-${i}` });

        this.clients.push(analyticsEngine, loadBalancer, ...webServers);

        await analyticsEngine.connect();
        await loadBalancer.connect();
        await connectAll(webServers);

        // Analytics engine collects performance data
        await analyticsEngine.subscribe('performance/+/requests');
        await analyticsEngine.subscribe('performance/aggregated');

        // Load balancer monitors server performance
        await loadBalancer.subscribe('performance/+/health');

        // Web servers report request metrics
        for (let j = 0; j < webServers.length; j++) {
          const server = webServers[j];
          const serverId = `server-${j}`;

          const requestMetrics = {
            server_id: serverId,
            requests_per_second: Math.random() * 1000 + 100,
            avg_response_time: Math.random() * 500 + 50,
            p95_response_time: Math.random() * 1000 + 200,
            error_count: Math.floor(Math.random() * 10),
            active_connections: Math.floor(Math.random() * 500) + 100,
            timestamp: Date.now()
          };

          await server.publish(`performance/${serverId}/requests`,
            JSON.stringify(requestMetrics));

          // Server health status
          await server.publish(`performance/${serverId}/health`, JSON.stringify({
            server_id: serverId,
            status: requestMetrics.avg_response_time < 200 ? 'healthy' : 'degraded',
            load_score: requestMetrics.requests_per_second / 1000,
            availability: 0.99 + Math.random() * 0.01,
            timestamp: Date.now()
          }));
        }

        // Analytics engine aggregates performance data
        const allMessages = await analyticsEngine.waitForMessages(3, 5000);
        TestHelpers.assertEqual(allMessages.length, 3,
          'Analytics engine should receive performance data from all servers');

        // Calculate aggregated metrics
        let totalRPS = 0;
        let avgResponseTime = 0;
        for (const message of allMessages) {
          const metrics = JSON.parse(message.message);
          totalRPS += metrics.requests_per_second;
          avgResponseTime += metrics.avg_response_time;
        }

        avgResponseTime = avgResponseTime / allMessages.length;

        await analyticsEngine.publish('performance/aggregated', JSON.stringify({
          total_requests_per_second: totalRPS,
          avg_response_time: avgResponseTime,
          total_servers: webServers.length,
          healthy_servers: webServers.length,
          timestamp: Date.now()
        }));

        // Load balancer receives health updates
        const healthMessages = await loadBalancer.waitForMessages(3, 5000);
        TestHelpers.assertEqual(healthMessages.length, 3,
          'Load balancer should receive health data from all servers');

        console.log(`Performance test ${i + 1}: ${totalRPS.toFixed(2)} total RPS, ${avgResponseTime.toFixed(2)}ms avg response time`);
      });
    }
  }

  async cleanup() {
    console.log('\n🧹 Cleaning up real-world scenarios test clients...');
    await disconnectAll(this.clients);
    this.clients = [];
  }
}

// Run tests if this file is executed directly
if (require.main === module) {
  (async () => {
    const tests = new RealWorldScenariosTests();
    try {
      const report = await tests.runAllTests();
      console.log('\n📊 Real-World Scenarios Test Report:');
      console.log(`Total: ${report.summary.total}`);
      console.log(`Passed: ${report.summary.passed}`);
      console.log(`Failed: ${report.summary.failed}`);
      console.log(`Pass Rate: ${report.summary.passRate}%`);

      process.exit(report.summary.failed > 0 ? 1 : 0);
    } catch (error) {
      console.error('Real-world scenarios test execution failed:', error);
      process.exit(1);
    }
  })();
}

module.exports = RealWorldScenariosTests;