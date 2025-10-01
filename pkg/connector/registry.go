// Copyright 2023 The emqx-go Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package connector

// RegisterDefaultFactories registers all default connector factories with the manager
func RegisterDefaultFactories(manager *ConnectorManager) {
	// Register HTTP connector factory
	manager.RegisterFactory(&HTTPConnectorFactory{})

	// Register MySQL connector factory
	manager.RegisterFactory(&MySQLConnectorFactory{})

	// Register PostgreSQL connector factory
	manager.RegisterFactory(&PostgreSQLConnectorFactory{})

	// Register Redis connector factory
	manager.RegisterFactory(&RedisConnectorFactory{})

	// Register Kafka connector factory
	manager.RegisterFactory(&KafkaConnectorFactory{})
}

// GetRegisteredConnectorTypes returns a list of all registered connector types
func GetRegisteredConnectorTypes() []ConnectorType {
	return []ConnectorType{
		ConnectorTypeHTTP,
		ConnectorTypeMySQL,
		ConnectorTypePostgreSQL,
		ConnectorTypeRedis,
		ConnectorTypeKafka,
	}
}

// CreateDefaultConnectorManager creates a new connector manager with all default factories registered
func CreateDefaultConnectorManager() *ConnectorManager {
	manager := NewConnectorManager()
	RegisterDefaultFactories(manager)
	return manager
}