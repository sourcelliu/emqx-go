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

// Package concurrency_tests provides comprehensive concurrent benchmarking for emqx-go
package concurrency_tests

import (
	"context"
	"fmt"
	"testing"

	"github.com/turtacn/emqx-go/pkg/actor"
	"github.com/turtacn/emqx-go/pkg/storage"
	"github.com/turtacn/emqx-go/pkg/topic"
)

// BenchmarkTopicStoreSubscribe measures topic subscription performance
func BenchmarkTopicStoreSubscribe(b *testing.B) {
	store := topic.NewStore()
	mailbox := actor.NewMailbox(100)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			topicName := fmt.Sprintf("bench/topic/%d", i%1000)
			store.Subscribe(topicName, mailbox, 1)
			i++
		}
	})
}

// BenchmarkTopicStoreGetSubscribers measures subscriber lookup performance
func BenchmarkTopicStoreGetSubscribers(b *testing.B) {
	store := topic.NewStore()
	mailbox := actor.NewMailbox(100)

	// Pre-populate with subscribers
	for i := 0; i < 1000; i++ {
		topicName := fmt.Sprintf("bench/topic/%d", i)
		store.Subscribe(topicName, mailbox, 1)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			topicName := fmt.Sprintf("bench/topic/%d", i%1000)
			_ = store.GetSubscribers(topicName)
			i++
		}
	})
}

// BenchmarkTopicStoreMixed measures mixed operations performance
func BenchmarkTopicStoreMixed(b *testing.B) {
	store := topic.NewStore()
	mailbox := actor.NewMailbox(100)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			topicName := fmt.Sprintf("bench/topic/%d", i%1000)

			switch i % 10 {
			case 0, 1, 2, 3, 4: // 50% subscribe
				store.Subscribe(topicName, mailbox, 1)
			case 5, 6, 7: // 30% lookup
				_ = store.GetSubscribers(topicName)
			case 8, 9: // 20% unsubscribe
				store.Unsubscribe(topicName, mailbox)
			}
			i++
		}
	})
}

// BenchmarkStorageSet measures storage set performance
func BenchmarkStorageSet(b *testing.B) {
	store := storage.NewMemStore()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("bench-key-%d", i)
			value := fmt.Sprintf("bench-value-%d", i)
			store.Set(key, value)
			i++
		}
	})
}

// BenchmarkStorageGet measures storage get performance
func BenchmarkStorageGet(b *testing.B) {
	store := storage.NewMemStore()

	// Pre-populate with data
	for i := 0; i < 10000; i++ {
		key := fmt.Sprintf("bench-key-%d", i)
		value := fmt.Sprintf("bench-value-%d", i)
		store.Set(key, value)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("bench-key-%d", i%10000)
			_, _ = store.Get(key)
			i++
		}
	})
}

// BenchmarkStorageMixed measures mixed storage operations performance
func BenchmarkStorageMixed(b *testing.B) {
	store := storage.NewMemStore()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("bench-key-%d", i%10000)
			value := fmt.Sprintf("bench-value-%d", i)

			switch i % 10 {
			case 0, 1, 2: // 30% set
				store.Set(key, value)
			case 3, 4, 5, 6, 7, 8: // 60% get
				_, _ = store.Get(key)
			case 9: // 10% delete
				store.Delete(key)
			}
			i++
		}
	})
}

// BenchmarkActorMailboxSend measures mailbox send performance
func BenchmarkActorMailboxSend(b *testing.B) {
	mailbox := actor.NewMailbox(10000) // Large buffer to avoid blocking
	ctx := context.Background()

	// Start a consumer to drain the mailbox
	go func() {
		for {
			_, err := mailbox.Receive(ctx)
			if err != nil {
				return
			}
		}
	}()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			message := fmt.Sprintf("message-%d", i)
			mailbox.Send(message)
			i++
		}
	})
}

// BenchmarkActorMailboxReceive measures mailbox receive performance
func BenchmarkActorMailboxReceive(b *testing.B) {
	mailbox := actor.NewMailbox(10000)
	ctx := context.Background()

	// Pre-fill mailbox with messages
	for i := 0; i < 10000; i++ {
		mailbox.Send(fmt.Sprintf("message-%d", i))
	}

	// Keep refilling in background
	go func() {
		i := 10000
		for {
			mailbox.Send(fmt.Sprintf("message-%d", i))
			i++
		}
	}()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = mailbox.Receive(ctx)
		}
	})
}

// BenchmarkConcurrentTopicOperations measures concurrent topic operations
func BenchmarkConcurrentTopicOperations(b *testing.B) {
	store := topic.NewStore()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		mailbox := actor.NewMailbox(100)
		i := 0
		for pb.Next() {
			topicName := fmt.Sprintf("concurrent/topic/%d", i%100)

			// Simulate realistic usage pattern
			store.Subscribe(topicName, mailbox, 1)
			subs := store.GetSubscribers(topicName)
			_ = len(subs)

			if i%10 == 0 {
				store.Unsubscribe(topicName, mailbox)
			}
			i++
		}
	})
}

// BenchmarkHighContentionScenario simulates high contention scenarios
func BenchmarkHighContentionScenario(b *testing.B) {
	store := topic.NewStore()
	const hotTopic = "hot/topic"

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		mailbox := actor.NewMailbox(100)
		i := 0
		for pb.Next() {
			switch i % 5 {
			case 0, 1: // 40% subscribe to hot topic
				store.Subscribe(hotTopic, mailbox, 1)
			case 2, 3: // 40% lookup hot topic
				_ = store.GetSubscribers(hotTopic)
			case 4: // 20% unsubscribe from hot topic
				store.Unsubscribe(hotTopic, mailbox)
			}
			i++
		}
	})
}

// BenchmarkMessageStoreOperations measures message storage operations
func BenchmarkMessageStoreOperations(b *testing.B) {
	// This would benchmark our message storage implementation
	// For now, we'll use the basic storage as a placeholder
	store := storage.NewMemStore()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("msg-%d", i)
			message := map[string]interface{}{
				"topic":   fmt.Sprintf("bench/topic/%d", i%100),
				"payload": fmt.Sprintf("payload-%d", i),
				"qos":     byte(i % 3),
			}

			store.Set(key, message)
			_, _ = store.Get(key)

			if i%100 == 0 {
				store.Delete(key)
			}
			i++
		}
	})
}

// BenchmarkConcurrentActorSystems measures actor system performance under load
func BenchmarkConcurrentActorSystems(b *testing.B) {
	const numActors = 10
	mailboxes := make([]*actor.Mailbox, numActors)

	for i := range mailboxes {
		mailboxes[i] = actor.NewMailbox(1000)
	}

	// Start consumers for each mailbox
	ctx := context.Background()
	for _, mb := range mailboxes {
		go func(mailbox *actor.Mailbox) {
			for {
				_, err := mailbox.Receive(ctx)
				if err != nil {
					return
				}
			}
		}(mb)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			mailbox := mailboxes[i%numActors]
			message := fmt.Sprintf("actor-message-%d", i)
			mailbox.Send(message)
			i++
		}
	})
}

// BenchmarkMemoryAllocation measures memory allocation patterns
func BenchmarkMemoryAllocation(b *testing.B) {
	store := topic.NewStore()

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			// Operations that might cause allocations
			mailbox := actor.NewMailbox(10)
			topicName := fmt.Sprintf("alloc/topic/%d", i%1000)

			store.Subscribe(topicName, mailbox, 1)
			subs := store.GetSubscribers(topicName)
			_ = len(subs)

			i++
		}
	})
}