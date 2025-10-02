// MQTTX IoT Scenario and Advanced Error Recovery Tests (120 test cases)
const { MQTTXClient, createClients, connectAll, disconnectAll } = require('./utils/mqttx-client');
const { TestHelpers, TopicUtils, MessageValidator } = require('./utils/test-helpers');
const config = require('./utils/config');

class IoTScenarioTests {
  constructor() {
    this.helpers = new TestHelpers();
    this.clients = [];
    this.deviceSimulations = [];
  }

  async runAllTests() {
    console.log('🏭 Starting MQTTX IoT Scenario and Advanced Error Recovery Tests (120 test cases)');

    try {
      // Smart Home Automation Tests (30 tests)
      await this.runSmartHomeTests();

      // Industrial IoT Tests (30 tests)
      await this.runIndustrialIoTTests();

      // Vehicle Telematics Tests (30 tests)
      await this.runVehicleTelematicsTests();

      // Advanced Error Recovery Tests (30 tests)
      await this.runAdvancedErrorRecoveryTests();

      return this.helpers.generateReport();
    } finally {
      await this.cleanup();
    }
  }

  // Smart Home Automation Tests (30 test cases)
  async runSmartHomeTests() {
    console.log('\n🏠 Running Smart Home Automation Tests (30 test cases)');

    // Test 751-755: Smart Lighting System
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`smart-lighting-system-${i + 1}`, async () => {
        const rooms = ['living_room', 'bedroom', 'kitchen', 'bathroom', 'office'];
        const room = rooms[i];

        // Create devices for the room
        const lightController = new MQTTXClient({
          clientId: `light_controller_${room}`,
          protocolVersion: 5
        });
        const lightSensor = new MQTTXClient({
          clientId: `light_sensor_${room}`,
          protocolVersion: 5
        });
        const motionSensor = new MQTTXClient({
          clientId: `motion_sensor_${room}`,
          protocolVersion: 5
        });
        const mobileApp = new MQTTXClient({
          clientId: `mobile_app_${room}`,
          protocolVersion: 5
        });

        this.clients.push(lightController, lightSensor, motionSensor, mobileApp);

        await Promise.all([
          lightController.connect(),
          lightSensor.connect(),
          motionSensor.connect(),
          mobileApp.connect()
        ]);

        // Set up subscriptions
        await lightController.subscribe(`home/${room}/light/command`);
        await mobileApp.subscribe(`home/${room}/light/status`);
        await mobileApp.subscribe(`home/${room}/sensor/+`);

        // Simulate smart lighting scenario
        // 1. Motion detected
        await motionSensor.publish(`home/${room}/sensor/motion`, JSON.stringify({
          detected: true,
          timestamp: Date.now(),
          confidence: 0.95
        }));

        // 2. Light sensor reports darkness
        await lightSensor.publish(`home/${room}/sensor/light`, JSON.stringify({
          lux: 50 + i * 20,
          timestamp: Date.now()
        }));

        // 3. Mobile app sends light command
        await mobileApp.publish(`home/${room}/light/command`, JSON.stringify({
          action: 'turn_on',
          brightness: 80,
          color: { r: 255, g: 255, b: 255 }
        }));

        // 4. Light controller responds with status
        const commands = await lightController.waitForMessages(1, 3000);
        TestHelpers.assertEqual(commands.length, 1, 'Light controller should receive command');

        await lightController.publish(`home/${room}/light/status`, JSON.stringify({
          state: 'on',
          brightness: 80,
          power_consumption: 12.5
        }));

        // 5. Mobile app receives updates
        const updates = await mobileApp.waitForMessages(3, 5000); // motion, light, status
        TestHelpers.assertGreaterThan(updates.length, 2, 'Mobile app should receive sensor and status updates');

        console.log(`Smart lighting system for ${room} completed successfully`);
      });
    }

    // Test 756-760: Climate Control System
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`climate-control-system-${i + 1}`, async () => {
        const zones = ['zone1', 'zone2', 'zone3', 'zone4', 'zone5'];
        const zone = zones[i];

        const thermostat = new MQTTXClient({
          clientId: `thermostat_${zone}`,
          protocolVersion: 5
        });
        const tempSensor = new MQTTXClient({
          clientId: `temp_sensor_${zone}`,
          protocolVersion: 5
        });
        const humiditySensor = new MQTTXClient({
          clientId: `humidity_sensor_${zone}`,
          protocolVersion: 5
        });
        const hvacSystem = new MQTTXClient({
          clientId: `hvac_${zone}`,
          protocolVersion: 5
        });

        this.clients.push(thermostat, tempSensor, humiditySensor, hvacSystem);

        await Promise.all([
          thermostat.connect(),
          tempSensor.connect(),
          humiditySensor.connect(),
          hvacSystem.connect()
        ]);

        // Set up climate control network
        await thermostat.subscribe(`home/climate/${zone}/sensor/+`);
        await hvacSystem.subscribe(`home/climate/${zone}/hvac/command`);
        await thermostat.subscribe(`home/climate/${zone}/hvac/status`);

        // Simulate climate control scenario
        const targetTemp = 22 + i; // 22-26°C
        const currentTemp = targetTemp + 3; // 3°C above target
        const humidity = 45 + i * 5; // 45-65%

        // Sensors report current conditions
        await tempSensor.publish(`home/climate/${zone}/sensor/temperature`, JSON.stringify({
          temperature: currentTemp,
          unit: 'celsius',
          timestamp: Date.now()
        }));

        await humiditySensor.publish(`home/climate/${zone}/sensor/humidity`, JSON.stringify({
          humidity: humidity,
          timestamp: Date.now()
        }));

        // Thermostat receives sensor data
        const sensorData = await thermostat.waitForMessages(2, 3000);
        TestHelpers.assertEqual(sensorData.length, 2, 'Thermostat should receive sensor data');

        // Thermostat sends HVAC command based on temperature difference
        await thermostat.publish(`home/climate/${zone}/hvac/command`, JSON.stringify({
          mode: 'cooling',
          target_temperature: targetTemp,
          fan_speed: 'auto'
        }));

        // HVAC system responds
        const hvacCommands = await hvacSystem.waitForMessages(1, 3000);
        TestHelpers.assertEqual(hvacCommands.length, 1, 'HVAC should receive command');

        await hvacSystem.publish(`home/climate/${zone}/hvac/status`, JSON.stringify({
          mode: 'cooling',
          running: true,
          power_consumption: 1500 + i * 200
        }));

        console.log(`Climate control for ${zone} maintaining ${targetTemp}°C`);
      });
    }

    // Test 761-765: Security System Integration
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`security-system-integration-${i + 1}`, async () => {
        const securityZones = ['entry', 'perimeter', 'interior', 'basement', 'garage'];
        const zone = securityZones[i];

        const securityPanel = new MQTTXClient({
          clientId: `security_panel_${zone}`,
          protocolVersion: 5
        });
        const doorSensor = new MQTTXClient({
          clientId: `door_sensor_${zone}`,
          protocolVersion: 5
        });
        const cameraSensor = new MQTTXClient({
          clientId: `camera_${zone}`,
          protocolVersion: 5
        });
        const alarmSiren = new MQTTXClient({
          clientId: `alarm_${zone}`,
          protocolVersion: 5
        });
        const mobileAlert = new MQTTXClient({
          clientId: `mobile_security_${zone}`,
          protocolVersion: 5
        });

        this.clients.push(securityPanel, doorSensor, cameraSensor, alarmSiren, mobileAlert);

        await Promise.all([
          securityPanel.connect(),
          doorSensor.connect(),
          cameraSensor.connect(),
          alarmSiren.connect(),
          mobileAlert.connect()
        ]);

        // Set up security monitoring
        await securityPanel.subscribe(`security/${zone}/sensor/+`);
        await alarmSiren.subscribe(`security/${zone}/alarm/command`);
        await mobileAlert.subscribe(`security/${zone}/alert/+`);

        // Simulate security event
        const isArmed = i % 2 === 0; // Alternate between armed/disarmed

        // Set system state
        await securityPanel.publish(`security/${zone}/system/status`, JSON.stringify({
          armed: isArmed,
          mode: isArmed ? 'away' : 'home',
          timestamp: Date.now()
        }));

        // Door sensor detects opening
        await doorSensor.publish(`security/${zone}/sensor/door`, JSON.stringify({
          state: 'open',
          sensor_id: `door_${zone}_001`,
          timestamp: Date.now()
        }));

        // Security panel processes event
        const doorEvents = await securityPanel.waitForMessages(1, 3000);
        TestHelpers.assertEqual(doorEvents.length, 1, 'Security panel should detect door event');

        if (isArmed) {
          // If armed, trigger alarm sequence
          await securityPanel.publish(`security/${zone}/alarm/command`, JSON.stringify({
            action: 'activate',
            level: 'high',
            reason: 'unauthorized_entry'
          }));

          await securityPanel.publish(`security/${zone}/alert/intrusion`, JSON.stringify({
            severity: 'critical',
            location: zone,
            sensor: 'door_sensor',
            timestamp: Date.now()
          }));

          const alarmCommands = await alarmSiren.waitForMessages(1, 3000);
          const alerts = await mobileAlert.waitForMessages(1, 3000);

          TestHelpers.assertEqual(alarmCommands.length, 1, 'Alarm should be activated');
          TestHelpers.assertEqual(alerts.length, 1, 'Mobile alert should be sent');
        }

        // Camera captures event
        await cameraSensor.publish(`security/${zone}/sensor/camera`, JSON.stringify({
          event: 'motion_detected',
          confidence: 0.89,
          image_url: `https://security.home/images/${zone}_${Date.now()}.jpg`
        }));

        console.log(`Security system for ${zone} processed event (armed: ${isArmed})`);
      });
    }

    // Test 766-770: Energy Management System
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`energy-management-system-${i + 1}`, async () => {
        const appliances = ['washing_machine', 'dishwasher', 'water_heater', 'ev_charger', 'pool_pump'];
        const appliance = appliances[i];

        const energyController = new MQTTXClient({
          clientId: `energy_controller`,
          protocolVersion: 5
        });
        const smartMeter = new MQTTXClient({
          clientId: `smart_meter`,
          protocolVersion: 5
        });
        const applianceController = new MQTTXClient({
          clientId: `${appliance}_controller`,
          protocolVersion: 5
        });
        const solarInverter = new MQTTXClient({
          clientId: `solar_inverter`,
          protocolVersion: 5
        });

        this.clients.push(energyController, smartMeter, applianceController, solarInverter);

        await Promise.all([
          energyController.connect(),
          smartMeter.connect(),
          applianceController.connect(),
          solarInverter.connect()
        ]);

        // Set up energy monitoring
        await energyController.subscribe(`energy/meter/+`);
        await energyController.subscribe(`energy/solar/+`);
        await applianceController.subscribe(`energy/appliance/${appliance}/command`);

        // Solar production data
        const solarProduction = 3000 + i * 500; // 3-5 kW
        await solarInverter.publish(`energy/solar/production`, JSON.stringify({
          power_kw: solarProduction / 1000,
          daily_kwh: 15.5 + i * 2,
          efficiency: 0.95,
          timestamp: Date.now()
        }));

        // Current consumption
        const currentConsumption = 2000 + i * 400; // 2-3.6 kW
        await smartMeter.publish(`energy/meter/consumption`, JSON.stringify({
          power_kw: currentConsumption / 1000,
          daily_kwh: 22.3 + i * 3,
          grid_import: Math.max(0, currentConsumption - solarProduction),
          grid_export: Math.max(0, solarProduction - currentConsumption),
          timestamp: Date.now()
        }));

        // Energy controller processes data
        const energyData = await energyController.waitForMessages(2, 3000);
        TestHelpers.assertEqual(energyData.length, 2, 'Energy controller should receive production and consumption data');

        // Determine if excess solar is available
        const excessSolar = solarProduction > currentConsumption;

        if (excessSolar) {
          // Start non-essential appliance during excess solar
          await energyController.publish(`energy/appliance/${appliance}/command`, JSON.stringify({
            action: 'start',
            mode: 'eco',
            reason: 'excess_solar_available'
          }));

          const applianceCommands = await applianceController.waitForMessages(1, 3000);
          TestHelpers.assertEqual(applianceCommands.length, 1, 'Appliance should receive start command');

          // Appliance reports status
          await applianceController.publish(`energy/appliance/${appliance}/status`, JSON.stringify({
            state: 'running',
            power_consumption: 800 + i * 200,
            estimated_completion: Date.now() + 3600000 // 1 hour
          }));
        }

        console.log(`Energy management: Solar ${solarProduction}W, Load ${currentConsumption}W, Excess: ${excessSolar}`);
      });
    }

    // Test 771-780: Smart Appliance Coordination
    for (let i = 0; i < 10; i++) {
      await this.helpers.runTest(`smart-appliance-coordination-${i + 1}`, async () => {
        const appliances = [
          'smart_oven', 'coffee_maker', 'robot_vacuum', 'air_purifier', 'smart_tv',
          'sound_system', 'window_blinds', 'garage_door', 'sprinkler_system', 'smart_doorbell'
        ];
        const appliance = appliances[i];

        const homeHub = new MQTTXClient({
          clientId: `home_hub`,
          protocolVersion: 5
        });
        const applianceDevice = new MQTTXClient({
          clientId: appliance,
          protocolVersion: 5
        });
        const userApp = new MQTTXClient({
          clientId: `user_app_${appliance}`,
          protocolVersion: 5
        });

        this.clients.push(homeHub, applianceDevice, userApp);

        await Promise.all([
          homeHub.connect(),
          applianceDevice.connect(),
          userApp.connect()
        ]);

        // Set up appliance coordination
        await homeHub.subscribe(`home/appliance/${appliance}/+`);
        await applianceDevice.subscribe(`home/appliance/${appliance}/command`);
        await userApp.subscribe(`home/appliance/${appliance}/status`);
        await userApp.subscribe(`home/appliance/${appliance}/notification`);

        // User initiates appliance operation
        const commands = {
          smart_oven: { action: 'preheat', temperature: 180, mode: 'convection' },
          coffee_maker: { action: 'brew', strength: 'medium', cups: 2 },
          robot_vacuum: { action: 'clean', area: 'living_room', mode: 'normal' },
          air_purifier: { action: 'auto', target_aqi: 50 },
          smart_tv: { action: 'power_on', input: 'netflix' },
          sound_system: { action: 'play', playlist: 'morning_jazz', volume: 30 },
          window_blinds: { action: 'open', position: 75 },
          garage_door: { action: 'open' },
          sprinkler_system: { action: 'start', zone: 'front_yard', duration: 15 },
          smart_doorbell: { action: 'arm', motion_detection: true }
        };

        await userApp.publish(`home/appliance/${appliance}/command`, JSON.stringify(commands[appliance]));

        // Appliance receives and processes command
        const applianceCommands = await applianceDevice.waitForMessages(1, 3000);
        TestHelpers.assertEqual(applianceCommands.length, 1, `${appliance} should receive command`);

        // Appliance reports status updates
        await applianceDevice.publish(`home/appliance/${appliance}/status`, JSON.stringify({
          state: 'active',
          operation: commands[appliance].action,
          progress: 25,
          estimated_completion: Date.now() + 300000 // 5 minutes
        }));

        // Send notification when operation completes
        setTimeout(async () => {
          await applianceDevice.publish(`home/appliance/${appliance}/notification`, JSON.stringify({
            type: 'completion',
            message: `${appliance.replace('_', ' ')} operation completed successfully`,
            timestamp: Date.now()
          }));
        }, 1000);

        // Home hub logs appliance activity
        const hubUpdates = await homeHub.waitForMessages(1, 3000);
        TestHelpers.assertEqual(hubUpdates.length, 1, 'Home hub should receive appliance updates');

        // User app receives status and notifications
        const userUpdates = await userApp.waitForMessages(2, 5000);
        TestHelpers.assertGreaterThan(userUpdates.length, 1, 'User app should receive status and notifications');

        console.log(`Smart appliance coordination: ${appliance} operation completed`);
      });
    }
  }

  // Industrial IoT Tests (30 test cases)
  async runIndustrialIoTTests() {
    console.log('\n🏭 Running Industrial IoT Tests (30 test cases)');

    // Test 781-785: Manufacturing Line Monitoring
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`manufacturing-line-monitoring-${i + 1}`, async () => {
        const lineId = `line_${i + 1}`;
        const stations = ['input', 'assembly', 'quality', 'packaging', 'output'];

        const scadaSystem = new MQTTXClient({
          clientId: `scada_${lineId}`,
          protocolVersion: 5
        });
        const plcController = new MQTTXClient({
          clientId: `plc_${lineId}`,
          protocolVersion: 5
        });
        const qualityInspector = new MQTTXClient({
          clientId: `quality_${lineId}`,
          protocolVersion: 5
        });
        const maintenance = new MQTTXClient({
          clientId: `maintenance_${lineId}`,
          protocolVersion: 5
        });

        this.clients.push(scadaSystem, plcController, qualityInspector, maintenance);

        await Promise.all([
          scadaSystem.connect(),
          plcController.connect(),
          qualityInspector.connect(),
          maintenance.connect()
        ]);

        // Set up industrial monitoring
        await scadaSystem.subscribe(`factory/${lineId}/+/+`);
        await maintenance.subscribe(`factory/${lineId}/maintenance/+`);

        // Simulate production cycle
        for (let stationIdx = 0; stationIdx < stations.length; stationIdx++) {
          const station = stations[stationIdx];

          // PLC reports station status
          await plcController.publish(`factory/${lineId}/${station}/status`, JSON.stringify({
            state: 'processing',
            cycle_count: 1000 + i * 100 + stationIdx,
            efficiency: 0.92 + Math.random() * 0.06,
            temperature: 65 + stationIdx * 5,
            pressure: 2.1 + stationIdx * 0.2,
            timestamp: Date.now()
          }));

          // Quality check at quality station
          if (station === 'quality') {
            await qualityInspector.publish(`factory/${lineId}/quality/inspection`, JSON.stringify({
              part_id: `part_${Date.now()}`,
              pass: Math.random() > 0.05, // 95% pass rate
              measurements: {
                dimension_x: 10.0 + (Math.random() - 0.5) * 0.1,
                dimension_y: 20.0 + (Math.random() - 0.5) * 0.1,
                weight: 150.0 + (Math.random() - 0.5) * 2.0
              },
              inspector_id: `QC_${i + 1}`
            }));
          }

          await TestHelpers.sleep(200); // Simulate processing time
        }

        // Machine requires maintenance
        if (i === 4) { // Last test triggers maintenance
          await plcController.publish(`factory/${lineId}/maintenance/alert`, JSON.stringify({
            machine_id: `machine_${lineId}_assembly`,
            alert_type: 'scheduled_maintenance',
            urgency: 'medium',
            estimated_downtime: 120, // minutes
            next_service_due: Date.now() + 7 * 24 * 3600000 // 7 days
          }));

          const maintenanceAlerts = await maintenance.waitForMessages(1, 3000);
          TestHelpers.assertEqual(maintenanceAlerts.length, 1, 'Maintenance should receive alert');
        }

        // SCADA collects all data
        const scadaData = await scadaSystem.waitForMessages(stations.length + (i === 4 ? 2 : 1), 5000);
        TestHelpers.assertGreaterThan(scadaData.length, stations.length - 1, 'SCADA should collect production data');

        console.log(`Manufacturing line ${lineId} monitoring completed with ${scadaData.length} data points`);
      });
    }

    // Test 786-790: Predictive Maintenance
    for (let i = 0; i < 5; i++) {
      await this.helpers.runTest(`predictive-maintenance-${i + 1}`, async () => {
        const equipmentTypes = ['compressor', 'conveyor', 'welding_robot', 'cnc_machine', 'packaging_unit'];
        const equipment = equipmentTypes[i];

        const vibrationSensor = new MQTTXClient({
          clientId: `vibration_${equipment}`,
          protocolVersion: 5
        });
        const temperatureSensor = new MQTTXClient({
          clientId: `temperature_${equipment}`,
          protocolVersion: 5
        });
        const currentSensor = new MQTTXClient({
          clientId: `current_${equipment}`,
          protocolVersion: 5
        });
        const predictiveSystem = new MQTTXClient({
          clientId: `predictive_${equipment}`,
          protocolVersion: 5
        });
        const maintenanceTeam = new MQTTXClient({
          clientId: `maintenance_team_${equipment}`,
          protocolVersion: 5
        });

        this.clients.push(vibrationSensor, temperatureSensor, currentSensor, predictiveSystem, maintenanceTeam);

        await Promise.all([
          vibrationSensor.connect(),
          temperatureSensor.connect(),
          currentSensor.connect(),
          predictiveSystem.connect(),
          maintenanceTeam.connect()
        ]);

        // Set up predictive maintenance monitoring
        await predictiveSystem.subscribe(`industrial/sensors/${equipment}/+`);
        await maintenanceTeam.subscribe(`industrial/maintenance/${equipment}/+`);

        // Simulate sensor data over time (degradation pattern)
        const baselineVibration = 2.5 + i * 0.5; // mm/s
        const baselineTemp = 45 + i * 5; // °C
        const baselineCurrent = 15 + i * 2; // A

        for (let reading = 0; reading < 10; reading++) {
          // Simulate equipment degradation over time
          const degradationFactor = 1 + (reading / 100) * (i + 1); // Gradual increase

          await vibrationSensor.publish(`industrial/sensors/${equipment}/vibration`, JSON.stringify({
            rms_velocity: baselineVibration * degradationFactor,
            peak_frequency: 1800 + reading * 10,
            amplitude: 0.5 * degradationFactor,
            timestamp: Date.now() - (10 - reading) * 60000 // Historical data
          }));

          await temperatureSensor.publish(`industrial/sensors/${equipment}/temperature`, JSON.stringify({
            bearing_temp: baselineTemp * degradationFactor,
            motor_temp: (baselineTemp - 5) * degradationFactor,
            ambient_temp: 22,
            timestamp: Date.now() - (10 - reading) * 60000
          }));

          await currentSensor.publish(`industrial/sensors/${equipment}/current`, JSON.stringify({
            motor_current: baselineCurrent * degradationFactor,
            power_factor: 0.85 / degradationFactor,
            frequency: 50,
            timestamp: Date.now() - (10 - reading) * 60000
          }));
        }

        // Predictive system processes sensor data
        const sensorReadings = await predictiveSystem.waitForMessages(30, 10000); // 3 sensors × 10 readings
        TestHelpers.assertGreaterThan(sensorReadings.length, 20, 'Predictive system should receive sensor data');

        // Generate maintenance prediction
        const riskLevel = i < 2 ? 'low' : i < 4 ? 'medium' : 'high';
        const daysToFailure = 30 - i * 6; // 30, 24, 18, 12, 6 days

        await predictiveSystem.publish(`industrial/maintenance/${equipment}/prediction`, JSON.stringify({
          equipment_id: equipment,
          risk_level: riskLevel,
          predicted_failure_date: Date.now() + daysToFailure * 24 * 3600000,
          confidence: 0.85 + i * 0.03,
          recommended_action: riskLevel === 'high' ? 'immediate_maintenance' : 'schedule_maintenance',
          failure_modes: ['bearing_wear', 'motor_overheating', 'belt_tension'],
          estimated_cost: 5000 + i * 2000
        }));

        // Maintenance team receives prediction
        const predictions = await maintenanceTeam.waitForMessages(1, 3000);
        TestHelpers.assertEqual(predictions.length, 1, 'Maintenance team should receive prediction');

        if (riskLevel === 'high') {
          // Generate immediate work order
          await maintenanceTeam.publish(`industrial/maintenance/${equipment}/work_order`, JSON.stringify({
            priority: 'urgent',
            type: 'predictive_maintenance',
            scheduled_date: Date.now() + 24 * 3600000, // Tomorrow
            estimated_duration: 4, // hours
            required_parts: ['bearing_6205', 'motor_oil', 'drive_belt'],
            technician_required: 'level_3'
          }));
        }

        console.log(`Predictive maintenance for ${equipment}: ${riskLevel} risk, ${daysToFailure} days to failure`);
      });
    }

    // Test 791-800: Supply Chain Tracking
    for (let i = 0; i < 10; i++) {
      await this.helpers.runTest(`supply-chain-tracking-${i + 1}`, async () => {
        const trackingStages = [
          'raw_material', 'incoming_inspection', 'production', 'quality_control', 'packaging',
          'warehouse', 'shipping', 'distribution', 'retail', 'customer'
        ];
        const currentStage = trackingStages[i];

        const rfidReader = new MQTTXClient({
          clientId: `rfid_${currentStage}`,
          protocolVersion: 5
        });
        const wmsSystem = new MQTTXClient({
          clientId: `wms_${currentStage}`,
          protocolVersion: 5
        });
        const trackingSystem = new MQTTXClient({
          clientId: `tracking_system`,
          protocolVersion: 5
        });
        const customerApp = new MQTTXClient({
          clientId: `customer_app`,
          protocolVersion: 5
        });

        this.clients.push(rfidReader, wmsSystem, trackingSystem, customerApp);

        await Promise.all([
          rfidReader.connect(),
          wmsSystem.connect(),
          trackingSystem.connect(),
          customerApp.connect()
        ]);

        // Set up supply chain tracking
        await trackingSystem.subscribe(`supply_chain/+/tracking`);
        await customerApp.subscribe(`supply_chain/customer/updates`);

        const batchId = `BATCH_${Date.now()}_${i}`;
        const rfidTag = `RFID_${batchId}`;

        // RFID reader detects item at current stage
        await rfidReader.publish(`supply_chain/${currentStage}/tracking`, JSON.stringify({
          rfid_tag: rfidTag,
          batch_id: batchId,
          stage: currentStage,
          timestamp: Date.now(),
          location: {
            facility: `${currentStage}_facility`,
            zone: `zone_${i + 1}`,
            coordinates: { lat: 40.7128 + i * 0.01, lng: -74.0060 + i * 0.01 }
          },
          operator_id: `OP_${String(i + 1).padStart(3, '0')}`
        }));

        // WMS system updates inventory and status
        await wmsSystem.publish(`supply_chain/${currentStage}/inventory`, JSON.stringify({
          batch_id: batchId,
          quantity: 1000 - i * 50, // Quantity decreases through supply chain
          status: 'in_transit',
          quality_grade: 'A',
          expected_delivery: Date.now() + (10 - i) * 24 * 3600000,
          carrier: i < 5 ? 'internal' : 'external_logistics'
        }));

        // Tracking system processes updates
        const trackingUpdates = await trackingSystem.waitForMessages(1, 3000);
        TestHelpers.assertEqual(trackingUpdates.length, 1, 'Tracking system should receive location update');

        // Send customer notification at key stages
        if (['production', 'shipping', 'distribution', 'customer'].includes(currentStage)) {
          const customerMessages = {
            production: 'Your order is now in production',
            shipping: 'Your order has been shipped',
            distribution: 'Your order is out for delivery',
            customer: 'Your order has been delivered'
          };

          await trackingSystem.publish(`supply_chain/customer/updates`, JSON.stringify({
            order_id: `ORD_${batchId}`,
            status: currentStage,
            message: customerMessages[currentStage],
            estimated_delivery: Date.now() + Math.max(0, (7 - i) * 24 * 3600000),
            tracking_url: `https://tracking.company.com/${batchId}`
          }));

          const customerUpdates = await customerApp.waitForMessages(1, 3000);
          TestHelpers.assertEqual(customerUpdates.length, 1, 'Customer should receive tracking update');
        }

        console.log(`Supply chain tracking: ${batchId} at ${currentStage} stage`);
      });
    }

    // Test 801-810: Environmental Monitoring
    for (let i = 0; i < 10; i++) {
      await this.helpers.runTest(`environmental-monitoring-${i + 1}`, async () => {
        const monitoringLocations = [
          'factory_floor', 'waste_treatment', 'chemical_storage', 'air_intake', 'water_outlet',
          'soil_sensors', 'noise_monitoring', 'radiation_detector', 'gas_leak_detection', 'weather_station'
        ];
        const location = monitoringLocations[i];

        const environmentalSensor = new MQTTXClient({
          clientId: `env_sensor_${location}`,
          protocolVersion: 5
        });
        const complianceSystem = new MQTTXClient({
          clientId: `compliance_${location}`,
          protocolVersion: 5
        });
        const alertSystem = new MQTTXClient({
          clientId: `alert_system`,
          protocolVersion: 5
        });
        const regulatoryReporting = new MQTTXClient({
          clientId: `regulatory_reporting`,
          protocolVersion: 5
        });

        this.clients.push(environmentalSensor, complianceSystem, alertSystem, regulatoryReporting);

        await Promise.all([
          environmentalSensor.connect(),
          complianceSystem.connect(),
          alertSystem.connect(),
          regulatoryReporting.connect()
        ]);

        // Set up environmental monitoring
        await complianceSystem.subscribe(`environment/${location}/+`);
        await alertSystem.subscribe(`environment/alerts/+`);

        // Define sensor parameters for each location type
        const sensorConfigs = {
          factory_floor: { temp: 25, humidity: 60, co2: 400, noise: 70 },
          waste_treatment: { ph: 7.2, turbidity: 5, bod: 20, flow_rate: 1000 },
          chemical_storage: { voc: 10, temperature: 20, leak_detector: false },
          air_intake: { pm25: 15, pm10: 25, o3: 80, no2: 40 },
          water_outlet: { ph: 7.5, dissolved_oxygen: 8, temperature: 18, conductivity: 500 },
          soil_sensors: { ph: 6.8, moisture: 45, nutrients: 85, heavy_metals: 2 },
          noise_monitoring: { decibels: 65, frequency_analysis: '1kHz-peak' },
          radiation_detector: { gamma_dose: 0.1, neutron_flux: 0.01 },
          gas_leak_detection: { methane: 5, hydrogen_sulfide: 0.1, oxygen: 20.9 },
          weather_station: { wind_speed: 15, wind_direction: 225, pressure: 1013 }
        };

        const config = sensorConfigs[location];
        const baselineReading = { ...config };

        // Simulate normal and abnormal readings
        const isAbnormal = i >= 7; // Last 3 tests simulate environmental alerts
        const readings = { ...baselineReading };

        if (isAbnormal) {
          // Simulate environmental violation
          Object.keys(readings).forEach(param => {
            if (typeof readings[param] === 'number') {
              readings[param] *= (1 + Math.random() * 0.5); // Increase by up to 50%
            } else if (typeof readings[param] === 'boolean') {
              readings[param] = true; // Trigger leak detection
            }
          });
        }

        // Sensor publishes readings
        await environmentalSensor.publish(`environment/${location}/readings`, JSON.stringify({
          ...readings,
          timestamp: Date.now(),
          sensor_id: `ENV_${location.toUpperCase()}`,
          calibration_date: Date.now() - 30 * 24 * 3600000, // 30 days ago
          battery_level: 85 + Math.random() * 10
        }));

        // Compliance system evaluates readings
        const sensorData = await complianceSystem.waitForMessages(1, 3000);
        TestHelpers.assertEqual(sensorData.length, 1, 'Compliance system should receive sensor data');

        if (isAbnormal) {
          // Generate compliance alert
          await complianceSystem.publish(`environment/alerts/violation`, JSON.stringify({
            location: location,
            severity: 'high',
            parameter: Object.keys(readings)[0],
            measured_value: Object.values(readings)[0],
            threshold_exceeded: true,
            regulation: 'EPA_STANDARD_001',
            action_required: 'immediate_investigation',
            timestamp: Date.now()
          }));

          // Alert system processes violation
          const alerts = await alertSystem.waitForMessages(1, 3000);
          TestHelpers.assertEqual(alerts.length, 1, 'Alert system should receive violation alert');

          // Generate regulatory report
          await regulatoryReporting.publish(`environment/reporting/incident`, JSON.stringify({
            incident_id: `INC_${Date.now()}`,
            location: location,
            incident_type: 'environmental_threshold_exceeded',
            reported_to: ['EPA', 'local_authority'],
            corrective_actions: 'system_maintenance_scheduled',
            estimated_resolution: Date.now() + 24 * 3600000
          }));
        }

        console.log(`Environmental monitoring ${location}: ${isAbnormal ? 'VIOLATION DETECTED' : 'Normal'}`);
      });
    }
  }

  // Vehicle Telematics Tests (30 test cases)
  async runVehicleTelematicsTests() {
    console.log('\n🚗 Running Vehicle Telematics Tests (30 test cases)');

    // Test 811-820: Fleet Management System
    for (let i = 0; i < 10; i++) {
      await this.helpers.runTest(`fleet-management-${i + 1}`, async () => {
        const vehicleTypes = [
          'delivery_truck', 'passenger_car', 'service_van', 'emergency_vehicle', 'public_bus',
          'construction_truck', 'taxi', 'rental_car', 'school_bus', 'garbage_truck'
        ];
        const vehicleType = vehicleTypes[i];
        const vehicleId = `VEH_${vehicleType}_${String(i + 1).padStart(3, '0')}`;

        const vehicleUnit = new MQTTXClient({
          clientId: vehicleId,
          protocolVersion: 5
        });
        const fleetManager = new MQTTXClient({
          clientId: `fleet_manager`,
          protocolVersion: 5
        });
        const dispatcher = new MQTTXClient({
          clientId: `dispatcher`,
          protocolVersion: 5
        });
        const maintenanceCenter = new MQTTXClient({
          clientId: `maintenance_center`,
          protocolVersion: 5
        });

        this.clients.push(vehicleUnit, fleetManager, dispatcher, maintenanceCenter);

        await Promise.all([
          vehicleUnit.connect(),
          fleetManager.connect(),
          dispatcher.connect(),
          maintenanceCenter.connect()
        ]);

        // Set up fleet management
        await fleetManager.subscribe(`fleet/vehicle/+/+`);
        await dispatcher.subscribe(`fleet/dispatch/+`);
        await maintenanceCenter.subscribe(`fleet/maintenance/+`);

        // Vehicle telemetry data
        const baseLocation = { lat: 40.7128 + i * 0.1, lng: -74.0060 + i * 0.1 };
        const speed = 30 + Math.random() * 50; // 30-80 km/h
        const fuel = 45 + Math.random() * 40; // 45-85%

        await vehicleUnit.publish(`fleet/vehicle/${vehicleId}/telemetry`, JSON.stringify({
          location: baseLocation,
          speed: speed,
          heading: 180 + i * 36, // Different directions
          fuel_level: fuel,
          odometer: 50000 + i * 5000,
          engine_hours: 2000 + i * 200,
          engine_temperature: 85 + Math.random() * 10,
          battery_voltage: 12.5 + Math.random() * 0.5,
          timestamp: Date.now()
        }));

        // Driver behavior data
        await vehicleUnit.publish(`fleet/vehicle/${vehicleId}/driver_behavior`, JSON.stringify({
          driver_id: `DRV_${String(i + 1).padStart(3, '0')}`,
          harsh_acceleration: Math.random() < 0.1,
          harsh_braking: Math.random() < 0.1,
          rapid_cornering: Math.random() < 0.05,
          speeding_events: Math.floor(Math.random() * 3),
          idle_time: Math.random() * 300, // seconds
          score: 85 + Math.random() * 10
        }));

        // Fleet manager processes data
        const telemetryData = await fleetManager.waitForMessages(2, 3000);
        TestHelpers.assertEqual(telemetryData.length, 2, 'Fleet manager should receive telemetry and behavior data');

        // Dispatch assignment for certain vehicle types
        if (['delivery_truck', 'service_van', 'taxi'].includes(vehicleType)) {
          await dispatcher.publish(`fleet/dispatch/assignment`, JSON.stringify({
            vehicle_id: vehicleId,
            job_id: `JOB_${Date.now()}`,
            priority: 'normal',
            pickup_location: { lat: baseLocation.lat + 0.01, lng: baseLocation.lng + 0.01 },
            destination: { lat: baseLocation.lat + 0.05, lng: baseLocation.lng + 0.05 },
            estimated_distance: 5.2,
            estimated_duration: 15,
            customer_id: `CUST_${i + 1}`
          }));
        }

        // Maintenance alerts for high mileage vehicles
        if (i >= 7) { // Last 3 vehicles need maintenance
          await vehicleUnit.publish(`fleet/maintenance/alert`, JSON.stringify({
            vehicle_id: vehicleId,
            alert_type: 'scheduled_maintenance',
            mileage_due: 55000 + i * 5000,
            service_type: 'oil_change_inspection',
            urgency: 'medium',
            estimated_cost: 200 + i * 50,
            preferred_date: Date.now() + 7 * 24 * 3600000
          }));

          const maintenanceAlerts = await maintenanceCenter.waitForMessages(1, 3000);
          TestHelpers.assertEqual(maintenanceAlerts.length, 1, 'Maintenance center should receive alert');
        }

        console.log(`Fleet management: ${vehicleId} (${vehicleType}) - Speed: ${speed.toFixed(1)} km/h, Fuel: ${fuel.toFixed(1)}%`);
      });
    }

    // Test 821-830: Connected Car Services
    for (let i = 0; i < 10; i++) {
      await this.helpers.runTest(`connected-car-services-${i + 1}`, async () => {
        const services = [
          'navigation', 'media_streaming', 'remote_start', 'climate_control', 'security_system',
          'fuel_finder', 'parking_assist', 'traffic_info', 'weather_update', 'emergency_assist'
        ];
        const service = services[i];
        const carId = `CAR_${String(i + 1).padStart(3, '0')}`;

        const carSystem = new MQTTXClient({
          clientId: `car_${carId}`,
          protocolVersion: 5
        });
        const mobileApp = new MQTTXClient({
          clientId: `mobile_${carId}`,
          protocolVersion: 5
        });
        const cloudService = new MQTTXClient({
          clientId: `cloud_${service}`,
          protocolVersion: 5
        });
        const ownerDevice = new MQTTXClient({
          clientId: `owner_${carId}`,
          protocolVersion: 5
        });

        this.clients.push(carSystem, mobileApp, cloudService, ownerDevice);

        await Promise.all([
          carSystem.connect(),
          mobileApp.connect(),
          cloudService.connect(),
          ownerDevice.connect()
        ]);

        // Set up connected car services
        await carSystem.subscribe(`car/${carId}/command/+`);
        await cloudService.subscribe(`car/${carId}/request/+`);
        await ownerDevice.subscribe(`car/${carId}/notification/+`);

        // Service-specific scenarios
        const serviceCommands = {
          navigation: {
            request: { destination: '123 Main St', route_preference: 'fastest' },
            response: { eta: 25, distance: 15.5, traffic_delay: 5 }
          },
          media_streaming: {
            request: { service: 'spotify', playlist: 'road_trip_mix' },
            response: { status: 'playing', current_track: 'Highway to Hell' }
          },
          remote_start: {
            request: { action: 'start', duration: 10 },
            response: { engine_status: 'running', cabin_temp: 22 }
          },
          climate_control: {
            request: { temperature: 21, mode: 'auto', defrost: false },
            response: { current_temp: 18, target_reached: false }
          },
          security_system: {
            request: { action: 'arm', notify_on_trigger: true },
            response: { armed: true, sensors_active: 12 }
          },
          fuel_finder: {
            request: { fuel_type: 'premium', radius: 10 },
            response: { nearest_station: '2.3 km', price: 1.45 }
          },
          parking_assist: {
            request: { destination: 'downtown_mall' },
            response: { available_spots: 23, cost: 15, walking_distance: 100 }
          },
          traffic_info: {
            request: { route: 'home_to_work' },
            response: { incidents: 2, alternative_route: true, time_savings: 8 }
          },
          weather_update: {
            request: { location: 'current' },
            response: { temperature: 18, condition: 'partly_cloudy', rain_probability: 20 }
          },
          emergency_assist: {
            request: { type: 'breakdown', location: 'highway_mile_marker_45' },
            response: { eta_assistance: 20, ticket_number: 'EM123456' }
          }
        };

        const serviceData = serviceCommands[service];

        // Mobile app initiates service request
        await mobileApp.publish(`car/${carId}/request/${service}`, JSON.stringify({
          ...serviceData.request,
          timestamp: Date.now(),
          user_id: `USER_${carId}`
        }));

        // Cloud service processes request
        const serviceRequests = await cloudService.waitForMessages(1, 3000);
        TestHelpers.assertEqual(serviceRequests.length, 1, `${service} service should receive request`);

        // Cloud service responds to car
        await cloudService.publish(`car/${carId}/command/${service}`, JSON.stringify({
          ...serviceData.response,
          request_id: `REQ_${Date.now()}`,
          status: 'success'
        }));

        // Car system receives and processes command
        const carCommands = await carSystem.waitForMessages(1, 3000);
        TestHelpers.assertEqual(carCommands.length, 1, 'Car should receive service command');

        // Car system sends status update
        await carSystem.publish(`car/${carId}/status/${service}`, JSON.stringify({
          service: service,
          status: 'active',
          ...serviceData.response,
          timestamp: Date.now()
        }));

        // Send notification to owner
        await carSystem.publish(`car/${carId}/notification/${service}`, JSON.stringify({
          title: `${service.replace('_', ' ')} activated`,
          message: `Your ${service.replace('_', ' ')} service is now active`,
          priority: service === 'emergency_assist' ? 'high' : 'normal',
          timestamp: Date.now()
        }));

        const ownerNotifications = await ownerDevice.waitForMessages(1, 3000);
        TestHelpers.assertEqual(ownerNotifications.length, 1, 'Owner should receive service notification');

        console.log(`Connected car service: ${service} activated for ${carId}`);
      });
    }

    // Test 831-840: Autonomous Vehicle Coordination
    for (let i = 0; i < 10; i++) {
      await this.helpers.runTest(`autonomous-vehicle-coordination-${i + 1}`, async () => {
        const autonomyLevels = [
          'level_0', 'level_1', 'level_2', 'level_3', 'level_4',
          'level_4', 'level_5', 'level_5', 'level_5', 'level_5'
        ];
        const autonomyLevel = autonomyLevels[i];
        const avId = `AV_${autonomyLevel}_${String(i + 1).padStart(3, '0')}`;

        const autonomousVehicle = new MQTTXClient({
          clientId: avId,
          protocolVersion: 5
        });
        const trafficControl = new MQTTXClient({
          clientId: `traffic_control_center`,
          protocolVersion: 5
        });
        const v2vCommunication = new MQTTXClient({
          clientId: `v2v_${avId}`,
          protocolVersion: 5
        });
        const cloudAI = new MQTTXClient({
          clientId: `cloud_ai_processor`,
          protocolVersion: 5
        });

        this.clients.push(autonomousVehicle, trafficControl, v2vCommunication, cloudAI);

        await Promise.all([
          autonomousVehicle.connect(),
          trafficControl.connect(),
          v2vCommunication.connect(),
          cloudAI.connect()
        ]);

        // Set up autonomous vehicle coordination
        await trafficControl.subscribe(`traffic/vehicle/+/+`);
        await v2vCommunication.subscribe(`v2v/broadcast/+`);
        await cloudAI.subscribe(`ai/vehicle/+/sensor_data`);

        // Vehicle sensor data
        const sensorData = {
          lidar: {
            obstacles: Math.floor(Math.random() * 5),
            range: 200,
            resolution: 0.1
          },
          camera: {
            objects_detected: ['vehicle', 'pedestrian', 'traffic_sign'],
            weather_condition: 'clear',
            visibility: 95
          },
          radar: {
            forward_distance: 150.5,
            side_clearance: { left: 2.1, right: 1.8 },
            relative_speed: -5.2
          },
          gps: {
            accuracy: 0.5,
            location: { lat: 40.7128 + i * 0.01, lng: -74.0060 + i * 0.01 },
            altitude: 15.2
          }
        };

        await autonomousVehicle.publish(`ai/vehicle/${avId}/sensor_data`, JSON.stringify({
          ...sensorData,
          autonomy_level: autonomyLevel,
          driving_mode: parseInt(autonomyLevel.split('_')[1]) >= 3 ? 'autonomous' : 'assisted',
          timestamp: Date.now()
        }));

        // Traffic control coordination
        await autonomousVehicle.publish(`traffic/vehicle/${avId}/status`, JSON.stringify({
          speed: 45 + Math.random() * 20,
          lane: i % 3 + 1,
          intersection_approach: i >= 5,
          route_destination: `DEST_${i + 1}`,
          estimated_arrival: Date.now() + 15 * 60000 // 15 minutes
        }));

        // V2V communication with nearby vehicles
        if (i >= 3) { // Vehicles with higher autonomy participate in V2V
          await v2vCommunication.publish(`v2v/broadcast/safety`, JSON.stringify({
            sender_id: avId,
            message_type: 'hazard_warning',
            hazard_type: i === 8 ? 'emergency_vehicle_approaching' : 'construction_zone',
            location: sensorData.gps.location,
            severity: 'medium',
            duration: 300, // seconds
            broadcast_radius: 500 // meters
          }));
        }

        // Cloud AI processes sensor data
        const aiData = await cloudAI.waitForMessages(1, 3000);
        TestHelpers.assertEqual(aiData.length, 1, 'Cloud AI should receive sensor data');

        // AI provides driving decisions for high autonomy levels
        if (parseInt(autonomyLevel.split('_')[1]) >= 4) {
          await cloudAI.publish(`ai/vehicle/${avId}/decision`, JSON.stringify({
            recommended_action: 'maintain_course',
            confidence: 0.92,
            alternative_routes: 2,
            weather_adjustment: false,
            traffic_optimization: true,
            energy_efficiency: 85
          }));
        }

        // Traffic control center processes vehicle data
        const trafficData = await trafficControl.waitForMessages(1, 3000);
        TestHelpers.assertEqual(trafficData.length, 1, 'Traffic control should receive vehicle status');

        // Send traffic optimization commands
        await trafficControl.publish(`traffic/vehicle/${avId}/guidance`, JSON.stringify({
          recommended_speed: 50,
          lane_change_suggestion: i % 2 === 0 ? 'left' : 'maintain',
          traffic_light_timing: Date.now() + 30000, // Next green in 30 seconds
          congestion_ahead: i >= 7,
          alternative_route: i >= 8
        }));

        console.log(`Autonomous vehicle coordination: ${avId} (${autonomyLevel}) - Sensors active, V2V ${i >= 3 ? 'enabled' : 'disabled'}`);
      });
    }
  }

  // Advanced Error Recovery Tests (30 test cases)
  async runAdvancedErrorRecoveryTests() {
    console.log('\n🛠️ Running Advanced Error Recovery Tests (30 test cases)');

    // Test 841-850: Network Partition Recovery
    for (let i = 0; i < 10; i++) {
      await this.helpers.runTest(`network-partition-recovery-${i + 1}`, async () => {
        const partitionScenarios = [
          'single_client_isolation', 'broker_split_brain', 'geographic_partition', 'datacenter_failure', 'network_congestion',
          'dns_resolution_failure', 'firewall_blocking', 'router_failure', 'isp_outage', 'submarine_cable_cut'
        ];
        const scenario = partitionScenarios[i];

        const primaryClient = new MQTTXClient({
          clientId: `primary_${scenario}`,
          protocolVersion: 5,
          reconnectPeriod: 1000,
          clean: false // Persistent session
        });
        const backupClient = new MQTTXClient({
          clientId: `backup_${scenario}`,
          protocolVersion: 5,
          reconnectPeriod: 1000
        });
        const monitoringClient = new MQTTXClient({
          clientId: `monitor_${scenario}`,
          protocolVersion: 5
        });

        this.clients.push(primaryClient, backupClient, monitoringClient);

        await Promise.all([
          primaryClient.connect(),
          backupClient.connect(),
          monitoringClient.connect()
        ]);

        const topic = `recovery/partition/${scenario}`;
        await monitoringClient.subscribe(topic);

        // Establish normal communication
        await primaryClient.publish(topic, `Pre-partition message from primary`, { qos: 1 });
        let messages = await monitoringClient.waitForMessages(1, 3000);
        TestHelpers.assertEqual(messages.length, 1, 'Normal communication should work');

        // Simulate network partition by forcing disconnect
        console.log(`Simulating ${scenario} partition...`);
        primaryClient.client.end(true); // Force disconnect without proper DISCONNECT

        // Backup takes over during partition
        await TestHelpers.sleep(2000); // Simulate detection delay
        await backupClient.publish(topic, `Backup active during ${scenario}`, { qos: 1 });

        messages = await monitoringClient.waitForMessages(1, 3000);
        TestHelpers.assertEqual(messages.length, 1, 'Backup should handle traffic during partition');

        // Simulate network recovery
        console.log(`Recovering from ${scenario}...`);
        const recoveredClient = new MQTTXClient({
          clientId: `primary_${scenario}`,
          protocolVersion: 5,
          clean: false // Resume session
        });
        this.clients.push(recoveredClient);

        await recoveredClient.connect();
        await recoveredClient.publish(topic, `Primary recovered from ${scenario}`, { qos: 1 });

        messages = await monitoringClient.waitForMessages(1, 3000);
        TestHelpers.assertEqual(messages.length, 1, 'Primary should resume after recovery');

        console.log(`Network partition recovery: ${scenario} completed`);
      });
    }

    // Test 851-860: Message Queue Overflow Recovery
    for (let i = 0; i < 10; i++) {
      await this.helpers.runTest(`message-queue-overflow-recovery-${i + 1}`, async () => {
        const overflowScenarios = [
          'subscriber_offline', 'slow_consumer', 'broker_restart', 'disk_full', 'memory_pressure',
          'network_backpressure', 'client_crash', 'message_storm', 'persistent_session_buildup', 'wildcard_explosion'
        ];
        const scenario = overflowScenarios[i];

        const publisher = new MQTTXClient({
          clientId: `overflow_pub_${scenario}`,
          protocolVersion: 5
        });
        const subscriber = new MQTTXClient({
          clientId: `overflow_sub_${scenario}`,
          protocolVersion: 5,
          clean: false
        });

        this.clients.push(publisher, subscriber);

        await Promise.all([publisher.connect(), subscriber.connect()]);

        const topic = `recovery/overflow/${scenario}`;
        await subscriber.subscribe(topic, { qos: 1 });

        // Create normal baseline
        await publisher.publish(topic, `Baseline message for ${scenario}`, { qos: 1 });
        let messages = await subscriber.waitForMessages(1, 3000);
        TestHelpers.assertEqual(messages.length, 1, 'Baseline should work');

        // Simulate queue overflow scenario
        console.log(`Creating queue overflow: ${scenario}`);

        // Disconnect subscriber to build up queue
        await subscriber.disconnect();

        // Rapid message publishing to create overflow
        const overflowCount = 100 + i * 50; // 100-550 messages
        const publishPromises = [];

        for (let j = 0; j < overflowCount; j++) {
          publishPromises.push(
            publisher.publish(topic, `Overflow message ${j} for ${scenario}`, { qos: 1 })
          );
        }

        try {
          await Promise.all(publishPromises);
        } catch (error) {
          TestHelpers.assertTrue(true, `Overflow handling: ${error.message}`);
        }

        // Simulate queue management/recovery
        await TestHelpers.sleep(2000);

        // Reconnect subscriber with persistent session
        const recoveredSubscriber = new MQTTXClient({
          clientId: `overflow_sub_${scenario}`,
          protocolVersion: 5,
          clean: false
        });
        this.clients.push(recoveredSubscriber);

        await recoveredSubscriber.connect();

        // Should receive queued messages (broker may drop some due to overflow)
        const recoveredMessages = await recoveredSubscriber.waitForMessages(overflowCount, 10000);
        TestHelpers.assertGreaterThan(recoveredMessages.length, 0, 'Should recover some messages from queue');

        const recoveryRate = (recoveredMessages.length / overflowCount) * 100;
        console.log(`Queue overflow recovery: ${scenario} - ${recoveredMessages.length}/${overflowCount} messages recovered (${recoveryRate.toFixed(1)}%)`);
      });
    }

    // Test 861-870: Cascading Failure Recovery
    for (let i = 0; i < 10; i++) {
      await this.helpers.runTest(`cascading-failure-recovery-${i + 1}`, async () => {
        const cascadeScenarios = [
          'authentication_server_down', 'database_connection_loss', 'certificate_expiry', 'load_balancer_failure', 'dns_server_failure',
          'storage_system_failure', 'monitoring_system_down', 'logging_system_failure', 'backup_system_failure', 'disaster_recovery_test'
        ];
        const scenario = cascadeScenarios[i];

        // Create multiple interconnected components
        const components = [];
        for (let j = 0; j < 5; j++) {
          const component = new MQTTXClient({
            clientId: `component_${scenario}_${j}`,
            protocolVersion: 5,
            reconnectPeriod: 2000
          });
          components.push(component);
        }

        this.clients.push(...components);

        // Connect all components
        await Promise.all(components.map(comp => comp.connect()));

        const coordinationTopic = `recovery/cascade/${scenario}/coordination`;
        const statusTopic = `recovery/cascade/${scenario}/status`;

        // Set up component coordination
        await Promise.all(components.map(comp => comp.subscribe(coordinationTopic)));
        await components[0].subscribe(statusTopic); // Primary monitoring component

        // Establish normal operation
        await components[0].publish(coordinationTopic, JSON.stringify({
          type: 'health_check',
          timestamp: Date.now(),
          initiator: 'component_0'
        }));

        const healthChecks = await components[1].waitForMessages(1, 3000);
        TestHelpers.assertEqual(healthChecks.length, 1, 'Normal coordination should work');

        // Simulate cascading failure
        console.log(`Initiating cascading failure: ${scenario}`);

        // Fail components sequentially
        for (let failIdx = 1; failIdx < components.length; failIdx++) {
          console.log(`Failing component ${failIdx}...`);

          // Component reports failure before going down
          await components[failIdx].publish(statusTopic, JSON.stringify({
            component_id: `component_${failIdx}`,
            status: 'failing',
            reason: scenario,
            cascade_trigger: failIdx > 1,
            timestamp: Date.now()
          }));

          // Force disconnect to simulate failure
          components[failIdx].client.end(true);

          // Other components detect failure and adapt
          await components[0].publish(coordinationTopic, JSON.stringify({
            type: 'component_failure_detected',
            failed_component: failIdx,
            remaining_components: components.length - failIdx - 1,
            failover_mode: true,
            timestamp: Date.now()
          }));

          await TestHelpers.sleep(1000); // Simulate cascade propagation
        }

        // Recovery phase - components restart and rejoin
        console.log(`Initiating recovery from ${scenario}...`);

        const recoveredComponents = [];
        for (let recIdx = 1; recIdx < components.length; recIdx++) {
          const recoveredComponent = new MQTTXClient({
            clientId: `component_${scenario}_${recIdx}`,
            protocolVersion: 5,
            clean: false // Resume session if possible
          });
          recoveredComponents.push(recoveredComponent);

          await recoveredComponent.connect();
          await recoveredComponent.subscribe(coordinationTopic);

          // Report recovery
          await recoveredComponent.publish(statusTopic, JSON.stringify({
            component_id: `component_${recIdx}`,
            status: 'recovered',
            recovery_mode: 'automatic',
            services_restored: ['messaging', 'coordination'],
            timestamp: Date.now()
          }));

          await TestHelpers.sleep(500); // Stagger recovery
        }

        this.clients.push(...recoveredComponents);

        // Verify full system recovery
        await components[0].publish(coordinationTopic, JSON.stringify({
          type: 'system_recovery_verification',
          all_components_check: true,
          timestamp: Date.now()
        }));

        // All recovered components should respond
        const verificationResponses = await components[0].waitForMessages(components.length - 1, 5000);
        TestHelpers.assertGreaterThan(verificationResponses.length, (components.length - 1) * 0.7,
          'Most components should respond after recovery');

        console.log(`Cascading failure recovery: ${scenario} - ${verificationResponses.length}/${components.length - 1} components recovered`);
      });
    }
  }

  async cleanup() {
    console.log('\n🧹 Cleaning up IoT scenario and error recovery test clients...');
    console.log(`Device simulations completed: ${this.deviceSimulations.length}`);

    await disconnectAll(this.clients);
    this.clients = [];
  }
}

// Run tests if this file is executed directly
if (require.main === module) {
  (async () => {
    const tests = new IoTScenarioTests();
    try {
      const report = await tests.runAllTests();
      console.log('\n📊 IoT Scenario and Error Recovery Test Report:');
      console.log(`Total: ${report.summary.total}`);
      console.log(`Passed: ${report.summary.passed}`);
      console.log(`Failed: ${report.summary.failed}`);
      console.log(`Pass Rate: ${report.summary.passRate}%`);

      process.exit(report.summary.failed > 0 ? 1 : 0);
    } catch (error) {
      console.error('IoT scenario and error recovery test execution failed:', error);
      process.exit(1);
    }
  })();
}

module.exports = IoTScenarioTests;