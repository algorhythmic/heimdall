//go:build !linux

package application

import (
	"fmt"
	"heimdall/internal/adapters/hyprland"
	"heimdall/internal/model"
)

func supported() error                              { return fmt.Errorf("application launch requires Linux") }
func process(int) (model.ApplicationProcess, error) { return model.ApplicationProcess{}, supported() }
func launch(model.ApplicationSpec, string, *model.SessionBinding) hyprland.DispatchReceipt {
	return hyprland.DispatchReceipt{Detail: supported().Error()}
}
