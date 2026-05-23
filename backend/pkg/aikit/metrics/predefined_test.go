package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestEnable(t *testing.T) {
	Enable()
	assert.NotNil(t, GetHTTPRequestCounter())
	assert.NotNil(t, GetHTTPRequestDuration())
	assert.NotNil(t, GetCacheRequests())
	assert.NotNil(t, GetCircuitBreakerState())
	assert.NotNil(t, GetCircuitBreakerCalls())
	assert.NotNil(t, GetAsyncQueueEnqueueCounter())
	assert.NotNil(t, GetAsyncQueueConsumeCounter())
	assert.NotNil(t, GetAsyncQueueHandlerDuration())
	assert.NotNil(t, GetHTTPClientRequestCounter())
	assert.NotNil(t, GetHTTPClientRequestDuration())
	assert.NotNil(t, GetRedisRequestCounter())
	assert.NotNil(t, GetRedisRequestDuration())
	assert.NotNil(t, GetMySQLRequestCounter())
	assert.NotNil(t, GetMySQLRequestDuration())
}

func TestObserveHTTPRequest(t *testing.T) {
	assert.NotPanics(t, func() {
		ObserveHTTPRequest("GET", "/api/v1/users", 200, 100*time.Millisecond)
	})
}

func TestObserveCache(t *testing.T) {
	assert.NotPanics(t, func() {
		ObserveCache("user-cache", "l1", "hit")
		ObserveCache("user-cache", "l1", "miss")
	})
}

func TestObserveCircuitBreakerState(t *testing.T) {
	assert.NotPanics(t, func() {
		ObserveCircuitBreakerState("my-breaker", 0)
		ObserveCircuitBreakerState("my-breaker", 1)
		ObserveCircuitBreakerState("my-breaker", 2)
	})
}

func TestObserveCircuitBreakerCall(t *testing.T) {
	assert.NotPanics(t, func() {
		ObserveCircuitBreakerCall("my-breaker", "success")
		ObserveCircuitBreakerCall("my-breaker", "failure")
		ObserveCircuitBreakerCall("my-breaker", "rejected")
	})
}

func TestObserveAsyncQueueEnqueue(t *testing.T) {
	assert.NotPanics(t, func() {
		ObserveAsyncQueueEnqueue("/process", "success")
		ObserveAsyncQueueEnqueue("/process", "failure")
	})
}

func TestObserveAsyncQueueConsume(t *testing.T) {
	assert.NotPanics(t, func() {
		ObserveAsyncQueueConsume("/process", "success", 2*time.Second)
		ObserveAsyncQueueConsume("/process", "failure", 500*time.Millisecond)
	})
}

func TestObserveHTTPClientRequest(t *testing.T) {
	assert.NotPanics(t, func() {
		ObserveHTTPClientRequest("api-client", "POST", "/submit", "201", 150*time.Millisecond)
	})
}

func TestObserveMySQLQuery(t *testing.T) {
	assert.NotPanics(t, func() {
		ObserveMySQLQuery("main-db", "articles", "query", true, 10*time.Millisecond)
		ObserveMySQLQuery("main-db", "articles", "create", false, 50*time.Millisecond)
	})
}

