//go:build !darwin

package statusbar

type Bar struct{}

func New() *Bar         { return &Bar{} }
func (*Bar) Run()       {}
func (*Bar) Stop()      {}
func (*Bar) Set(string) {}
