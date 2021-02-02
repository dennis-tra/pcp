package config

import (
	"fmt"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dennis-tra/pcp/internal/app"
	"github.com/dennis-tra/pcp/internal/mock"
)

func setup(t *testing.T) *gomock.Controller {
	appXdg = app.Xdg{}
	appIoutil = app.Ioutil{}
	return gomock.NewController(t)
}

func teardown(t *testing.T, ctrl *gomock.Controller) {
	ctrl.Finish()
	appXdg = app.Xdg{}
	appIoutil = app.Ioutil{}
}

func TestLoadSettings_returnsErrorOfConfigFile(t *testing.T) {
	ctrl := setup(t)
	defer teardown(t, ctrl)

	m := mock.NewMockXdger(ctrl)

	expectedErr := fmt.Errorf("some error")
	m.EXPECT().
		ConfigFile(gomock.Any()).
		Return("", expectedErr)

	appXdg = m
	settings, err := LoadSettings()
	assert.Nil(t, settings)
	assert.Equal(t, expectedErr, err)
}

func TestLoadSettings_returnsErrorOfReadFile(t *testing.T) {
	ctrl := setup(t)
	defer teardown(t, ctrl)

	mioutil := mock.NewMockIoutiler(ctrl)
	mxdg := mock.NewMockXdger(ctrl)

	appIoutil = mioutil
	appXdg = mxdg

	mxdg.
		EXPECT().
		ConfigFile(gomock.Eq(settingsFile)).
		Return("path", nil)

	expectedErr := fmt.Errorf("some error")
	mioutil.
		EXPECT().
		ReadFile(gomock.Eq("path")).
		Return(nil, expectedErr)

	settings, err := LoadSettings()
	assert.Nil(t, settings)
	assert.Equal(t, expectedErr, err)
}

func TestLoadSettings_returnsErrorOnUnparsableData(t *testing.T) {
	ctrl := setup(t)
	defer teardown(t, ctrl)

	mioutil := mock.NewMockIoutiler(ctrl)
	mxdg := mock.NewMockXdger(ctrl)

	appIoutil = mioutil
	appXdg = mxdg

	mxdg.
		EXPECT().
		ConfigFile(gomock.Eq(settingsFile)).
		Return("path", nil)

	data := []byte(`unparsable`)
	mioutil.
		EXPECT().
		ReadFile(gomock.Eq("path")).
		Return(data, nil)

	settings, err := LoadSettings()
	assert.Nil(t, settings)
	assert.NotNil(t, err)
}

func TestLoadSettings_returnsEmptySettingsIfFileDoesNotExist(t *testing.T) {
	ctrl := setup(t)
	defer teardown(t, ctrl)

	mioutil := mock.NewMockIoutiler(ctrl)
	mxdg := mock.NewMockXdger(ctrl)

	appIoutil = mioutil
	appXdg = mxdg

	mxdg.
		EXPECT().
		ConfigFile(gomock.Eq(settingsFile)).
		Return("path", nil)

	mioutil.
		EXPECT().
		ReadFile(gomock.Eq("path")).
		Return(nil, os.ErrNotExist)

	settings, err := LoadSettings()
	require.NoError(t, err)
	assert.False(t, settings.Exists)
	assert.Equal(t, "path", settings.Path)
}

func TestLoadSettings_happyPath(t *testing.T) {
	ctrl := setup(t)
	defer teardown(t, ctrl)

	mioutil := mock.NewMockIoutiler(ctrl)
	mxdg := mock.NewMockXdger(ctrl)

	appIoutil = mioutil
	appXdg = mxdg

	mxdg.
		EXPECT().
		ConfigFile(gomock.Eq(settingsFile)).
		Return("path", nil)

	data := []byte(`{}`)
	mioutil.
		EXPECT().
		ReadFile(gomock.Eq("path")).
		Return(data, nil)

	settings, err := LoadSettings()
	require.NoError(t, err)
	assert.True(t, settings.Exists)
	assert.Equal(t, "path", settings.Path)
}
