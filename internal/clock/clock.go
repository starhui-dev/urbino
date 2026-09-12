package clock

import "time"

// Clock 抽象当前时间，业务代码和测试不得直接依赖全局时钟。
type Clock interface {
	Now() time.Time
}

type Real struct{}

func (Real) Now() time.Time { return time.Now() }
