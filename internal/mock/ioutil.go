// Code generated by MockGen. DO NOT EDIT.
// Source: internal/app/ioutil.go

// Package mock is a generated GoMock package.
package mock

import (
	os "os"
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
)

// MockIoutiler is a mock of Ioutiler interface
type MockIoutiler struct {
	ctrl     *gomock.Controller
	recorder *MockIoutilerMockRecorder
}

// MockIoutilerMockRecorder is the mock recorder for MockIoutiler
type MockIoutilerMockRecorder struct {
	mock *MockIoutiler
}

// NewMockIoutiler creates a new mock instance
func NewMockIoutiler(ctrl *gomock.Controller) *MockIoutiler {
	mock := &MockIoutiler{ctrl: ctrl}
	mock.recorder = &MockIoutilerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockIoutiler) EXPECT() *MockIoutilerMockRecorder {
	return m.recorder
}

// ReadFile mocks base method
func (m *MockIoutiler) ReadFile(filename string) ([]byte, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ReadFile", filename)
	ret0, _ := ret[0].([]byte)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ReadFile indicates an expected call of ReadFile
func (mr *MockIoutilerMockRecorder) ReadFile(filename interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReadFile", reflect.TypeOf((*MockIoutiler)(nil).ReadFile), filename)
}

// WriteFile mocks base method
func (m *MockIoutiler) WriteFile(filename string, data []byte, perm os.FileMode) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "WriteFile", filename, data, perm)
	ret0, _ := ret[0].(error)
	return ret0
}

// WriteFile indicates an expected call of WriteFile
func (mr *MockIoutilerMockRecorder) WriteFile(filename, data, perm interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "WriteFile", reflect.TypeOf((*MockIoutiler)(nil).WriteFile), filename, data, perm)
}
