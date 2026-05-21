package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSetFamily(t *testing.T) {
	SetFamily("my-service")
	assert.Equal(t, "my-service", ServiceFamily())
}

func TestServiceFamily_Default(t *testing.T) {
	serviceFamily.Store("")
	assert.Equal(t, "", ServiceFamily())
}

func TestGetHTTPRequestCounter(t *testing.T) {
	c := GetHTTPRequestCounter()
	assert.NotNil(t, c)
	assert.NotPanics(t, func() {
		c.Inc("test", "GET", "/api", "200")
	})
}

func TestGetHTTPRequestDuration(t *testing.T) {
	h := GetHTTPRequestDuration()
	assert.NotNil(t, h)
	assert.NotPanics(t, func() {
		h.Observe(0.5, "test", "GET", "/api")
	})
}

func TestGetCacheHits(t *testing.T) {
	c := GetCacheHits()
	assert.NotNil(t, c)
	assert.NotPanics(t, func() {
		c.Inc("test", "my-cache", "l1")
	})
}

func TestGetCacheMisses(t *testing.T) {
	c := GetCacheMisses()
	assert.NotNil(t, c)
	assert.NotPanics(t, func() {
		c.Inc("test", "my-cache")
	})
}

func TestGetCircuitBreakerState(t *testing.T) {
	g := GetCircuitBreakerState()
	assert.NotNil(t, g)
	assert.NotPanics(t, func() {
		g.Set(1.0, "test", "my-breaker")
	})
}

func TestGetCircuitBreakerCalls(t *testing.T) {
	c := GetCircuitBreakerCalls()
	assert.NotNil(t, c)
	assert.NotPanics(t, func() {
		c.Inc("test", "my-breaker", "success")
	})
}

func TestGetAsyncQueueEnqueueCounter(t *testing.T) {
	c := GetAsyncQueueEnqueueCounter()
	assert.NotNil(t, c)
	assert.NotPanics(t, func() {
		c.Inc("test", "/process", "success")
	})
}

func TestGetAsyncQueueConsumeCounter(t *testing.T) {
	c := GetAsyncQueueConsumeCounter()
	assert.NotNil(t, c)
	assert.NotPanics(t, func() {
		c.Inc("test", "/process", "success")
	})
}

func TestGetAsyncQueueHandlerDuration(t *testing.T) {
	h := GetAsyncQueueHandlerDuration()
	assert.NotNil(t, h)
	assert.NotPanics(t, func() {
		h.Observe(1.5, "test", "/process", "success")
	})
}

func TestGetHTTPClientRequestCounter(t *testing.T) {
	c := GetHTTPClientRequestCounter()
	assert.NotNil(t, c)
	assert.NotPanics(t, func() {
		c.Inc("test", "api-client", "GET", "/users", "200")
	})
}

func TestGetHTTPClientRequestDuration(t *testing.T) {
	h := GetHTTPClientRequestDuration()
	assert.NotNil(t, h)
	assert.NotPanics(t, func() {
		h.Observe(0.1, "test", "api-client", "GET", "/users")
	})
}

func TestGetMySQLRequestCounter(t *testing.T) {
	c := GetMySQLRequestCounter()
	assert.NotNil(t, c)
	assert.NotPanics(t, func() {
		c.Inc("test", "main-db", "SELECT", "true")
	})
}

func TestGetMySQLRequestDuration(t *testing.T) {
	h := GetMySQLRequestDuration()
	assert.NotNil(t, h)
	assert.NotPanics(t, func() {
		h.Observe(0.05, "test", "main-db", "SELECT")
	})
}

func TestObserveHTTPRequest(t *testing.T) {
	assert.NotPanics(t, func() {
		ObserveHTTPRequest("test", "GET", "/api/v1/users", 200, 100*time.Millisecond)
	})
}

func TestObserveCacheHit(t *testing.T) {
	assert.NotPanics(t, func() {
		ObserveCacheHit("test", "user-cache", "l1")
	})
}

func TestObserveCacheMiss(t *testing.T) {
	assert.NotPanics(t, func() {
		ObserveCacheMiss("test", "user-cache")
	})
}

func TestObserveCircuitBreakerState(t *testing.T) {
	assert.NotPanics(t, func() {
		ObserveCircuitBreakerState("test", "my-breaker", 0)
		ObserveCircuitBreakerState("test", "my-breaker", 1)
		ObserveCircuitBreakerState("test", "my-breaker", 2)
	})
}

func TestObserveCircuitBreakerCall(t *testing.T) {
	assert.NotPanics(t, func() {
		ObserveCircuitBreakerCall("test", "my-breaker", "success")
		ObserveCircuitBreakerCall("test", "my-breaker", "failure")
		ObserveCircuitBreakerCall("test", "my-breaker", "rejected")
	})
}

func TestObserveAsyncQueueEnqueue(t *testing.T) {
	assert.NotPanics(t, func() {
		ObserveAsyncQueueEnqueue("test", "/process", "success")
		ObserveAsyncQueueEnqueue("test", "/process", "failure")
	})
}

func TestObserveAsyncQueueConsume(t *testing.T) {
	assert.NotPanics(t, func() {
		ObserveAsyncQueueConsume("test", "/process", "success", 2*time.Second)
		ObserveAsyncQueueConsume("test", "/process", "failure", 500*time.Millisecond)
	})
}

func TestObserveHTTPClientRequest(t *testing.T) {
	assert.NotPanics(t, func() {
		ObserveHTTPClientRequest("test", "api-client", "POST", "/submit", "201", 150*time.Millisecond)
	})
}

func TestObserveMySQLQuery(t *testing.T) {
	assert.NotPanics(t, func() {
		ObserveMySQLQuery("test", "main-db", "SELECT", true, 10*time.Millisecond)
		ObserveMySQLQuery("test", "main-db", "INSERT", false, 50*time.Millisecond)
	})
}
