package pcptest

import tea "github.com/charmbracelet/bubbletea"

type VoidSender struct {
	msgChan chan tea.Msg
}

func (v VoidSender) Send(tea.Msg) {
}

var _ tea.Sender = (*VoidSender)(nil)
