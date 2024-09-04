package mstopper

import (
	"sync"
	"testing"
	"time"
)

func Test_Stopper(t *testing.T) {
	s := New()

	s.SetTimeout(time.Second)

	a := 0
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		m := s.NewModule()
		t.Logf("run module %d", i)
		wg.Add(1)
		go func(i int, mm *StopperModule) {
			defer wg.Done()
			mm.WaitStopTrigger()
			t.Logf("%d: got stop command", i)
			time.Sleep(time.Millisecond*50 + time.Duration(i)*50)
			time.Sleep(time.Second * 3)
			mm.Done()
			a++
			t.Logf("%d: done", i)
		}(i, m)

		wg.Add(1)
		go func() {
			defer wg.Done()
			t.Logf("waitStopC inited\n")
			m2 := s.NewModule()
			<-m2.WaitStopC()
			m2.Done()
			t.Logf("WaitStopC done")
		}()
	}
	time.Sleep(100 * time.Millisecond)

	t.Logf("call stop all")
	s.StopAll()

	wg.Wait()

	time.Sleep(time.Millisecond * 10)
	t.Logf("%d stopped", a)

	t.Logf("2 stop all")
	s.StopAll() //не вызывает зависание и повторный вызов останова у модулей
	t.Logf("2 stop all done")
}

func Test_Stopper2(t *testing.T) {
	s := New()

	s.SetTimeout(time.Second)

	t.Logf("1 stop all")

	s.StopAll()

	t.Logf("2 stop all")

	s.StopAll()

	t.Logf("3 stop all")
}
