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

import "errors"

// Common connector errors
var (
	ErrConnectorExists            = errors.New("connector already exists")
	ErrConnectorNotFound          = errors.New("connector not found")
	ErrConnectorTypeNotSupported  = errors.New("connector type not supported")
	ErrConnectorNotRunning        = errors.New("connector is not running")
	ErrConnectorAlreadyRunning    = errors.New("connector is already running")
	ErrConnectorConfiguration     = errors.New("invalid connector configuration")
	ErrConnectorConnection        = errors.New("connector connection failed")
	ErrConnectorTimeout           = errors.New("connector operation timeout")
	ErrConnectorRetryExhausted    = errors.New("connector retry attempts exhausted")
	ErrConnectorHealthCheckFailed = errors.New("connector health check failed")
	ErrConnectorClosed            = errors.New("connector is closed")
	ErrInvalidMessage             = errors.New("invalid message format")
	ErrMessageTooLarge            = errors.New("message too large")
	ErrRateLimited                = errors.New("rate limited")
)