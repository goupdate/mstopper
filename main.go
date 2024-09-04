package mstopper

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MasterDimmy/zipologger"
)

/*
	Информирует ожидающих, что пора завершать работу, т.к. был сигнал сверху
*/

type Stopper struct {
	wg      sync.WaitGroup
	ch_wait sync.Map
	ch_stop chan struct{}

	m       sync.Mutex
	timeout int64 //сколько времени в секундах ждать завершения работы модуля
	//до сообщения о проблеме

	stopCalled int32 //вызван стоп?
}

func New() *Stopper {
	return &Stopper{
		ch_stop: make(chan struct{}),
	}
}

func (s *Stopper) SetTimeout(d time.Duration) *Stopper {
	atomic.StoreInt64(&s.timeout, int64(d))
	return s
}

func (s *Stopper) IsStopCalled() bool {
	return atomic.LoadInt32(&s.stopCalled) == 1
}

// вызывается для передачи сигнала останова всем желающим
// ожидаем завершения работы всех, кто запросил стоппер
// похож на waitgroup с тем отличием, что помимо Wait дается команда всем на стоп
func (s *Stopper) StopAll() {
	s.m.Lock()
	defer s.m.Unlock()

	if atomic.CompareAndSwapInt32(&s.stopCalled, 0, 1) {
		close(s.ch_stop)
	}

	s.ch_wait.Range(func(k interface{}, v interface{}) bool {
		s.wg.Add(1)
		//сообщаем, что вызван стоп
		module := k.(*StopperModule)

		go func(ch chan bool, path string) { //ждем сообщения, что остановлен
			timeout := time.Duration(atomic.LoadInt64(&s.timeout))
			if timeout < time.Millisecond {
				timeout = time.Second * 10
			}
			t := time.NewTimer(timeout)
			select {
			case <-t.C:
				dmp := zipologger.GetLoggerBySuffix("dump_of_slow_module.log", "./logs/module_stopper", 2, 10, 30, false)
				buf := make([]byte, 10*1024*1024)
				n := runtime.Stack(buf, true)
				buf = buf[:n]
				dmp.Print(string(buf))
				dmp.Flush()
				fmt.Printf("module stopper: module %s is stopping too long. dump created\n", path)
			case <-ch:
			}
			s.wg.Done()
		}(module.ch, module.path)

		s.ch_wait.Delete(k)

		return true
	})
	s.wg.Wait()
}

//создаем модуль  s.NewModule()
//ждем команду на стоп mm.WaitStopTrigger()
//информируем вызывавшего останов, что мы остановились через mm.Done()
/*
 func Test_Stopper(t *testing.T) {
	s := New()

	a := 0
	for i := 0; i < 5; i++ {
		m := s.NewModule()
		t.Logf("run module %d", i)
		go func(i int, mm *StopperModule) {
			mm.WaitStopTrigger()
			t.Logf("%d: got stop command", i)
			time.Sleep(time.Millisecond*50 + time.Duration(i)*50)

			mm.Done()
			a++
			t.Logf("%d: done", i)
		}(i, m)
	}
	time.Sleep(100 * time.Millisecond)

	t.Logf("call stop all")
	s.StopAll()

	t.Logf("%d stopped", a)
	if a != 5 {
		t.Fail()
	}
}
*/

type StopperModule struct {
	path string
	ch   chan bool
	s    *Stopper
}

// зарегистрировать новый модуль, который будет ждать останова
func (s *Stopper) NewModule() *StopperModule {
	m := &StopperModule{
		ch:   make(chan bool, 1),
		s:    s,
		path: formatCaller(),
	}
	return m
}

// ждем команду останова
// defer WaitStopTrigger()
func (m *StopperModule) WaitStopTrigger() {
	m.s.ch_wait.Store(m, nil)

	<-m.s.ch_stop //ждем команды на стоп
}

// wait stop via  <-WaitStopC()
func (m *StopperModule) WaitStopC() <-chan bool {
	c := make(chan bool)
	go func() {
		mc := m.s.NewModule()
		mc.WaitStopTrigger()
		c <- true
		mc.Done()
	}()
	return c
}

// сообщаем наружу, что мы завершили работу
func (m *StopperModule) Done() {
	m.ch <- true
}

func formatCaller() string {
	type caller struct {
		f string
		l int
	}

	var callers []caller
	i := 0
	for {
		_, file, line, ok := runtime.Caller(i)
		i++
		if len(file) == 0 || !ok || line == 0 ||
			strings.HasSuffix(file, "runtime/proc.go") ||
			strings.HasSuffix(file, "src/testing/testing.go") ||
			strings.HasSuffix(file, "runtime/asm_amd64.s") {
			break
		}
		if len(callers) > 0 {
			if callers[len(callers)-1].f == file {
				file = ""
			}
		}
		callers = append(callers, caller{f: file, l: line})
	}

	ret := ""
	for i := 0; i < len(callers); i++ {
		if i > 0 {
			if callers[i].f != "" {
				ret += " => "
			}
		}
		ret = fmt.Sprintf("%s:%d", callers[i].f, callers[i].l)
	}

	return ret
}
