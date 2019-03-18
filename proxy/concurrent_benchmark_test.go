package proxy

import (
	"context"
	"testing"
	"time"

	"github.com/vm-affekt/krakend/config"
)

func BenchmarkNewConcurrentMiddleware_singleNext(b *testing.B) {
	backend := config.Backend{
		ConcurrentCalls: 3,
		Timeout:         time.Duration(100) * time.Millisecond,
	}
	proxy := NewConcurrentMiddleware(&backend)(dummyProxy(&Response{}))

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		proxy(context.Background(), &Request{})
	}
}
