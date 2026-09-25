package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/ushineko/ototo/internal/desktop"
)

// DesktopState is what ototo has installed into the desktop (D9, R8.5).
type DesktopState struct {
	Autostart     bool
	IndicatorRule bool
	VolumeKeys    bool
	// Errors are why a state could not be read, one per item, or nothing.
	Errors []string
}

// DesktopStatus reads the desktop steps' state.
func DesktopStatus(_ context.Context) DesktopState {
	var st DesktopState
	var err error
	if st.Autostart, err = desktop.AutostartInstalled(); err != nil {
		st.Errors = append(st.Errors, err.Error())
	}
	if st.IndicatorRule, err = desktop.RuleInstalled(); err != nil {
		st.Errors = append(st.Errors, err.Error())
	}
	if st.VolumeKeys, err = desktop.VolumeKeysInstalled(); err != nil {
		st.Errors = append(st.Errors, err.Error())
	}
	return st
}

// SetDesktopRequest changes one or more desktop steps. A nil field is left
// as it is, so one switch changes one thing.
type SetDesktopRequest struct {
	Request
	Autostart     *bool
	IndicatorRule *bool
	VolumeKeys    *bool
}

/*
SetDesktop installs or removes the desktop steps that are set, and reports
the state afterwards. Every step is done even when an earlier one fails, and
the errors come back together: a person who turned two things on wants to
know about both.
*/
func SetDesktop(ctx context.Context, req SetDesktopRequest) (DesktopState, error) {
	var errs []error
	if req.Autostart != nil {
		if *req.Autostart {
			if err := desktop.InstallAutostart(); err != nil {
				errs = append(errs, fmt.Errorf("autostart: %w", err))
			} else {
				req.Events.logf(LevelInfo, "autostart entry installed")
			}
		} else if _, err := desktop.RemoveAutostart(); err != nil {
			errs = append(errs, fmt.Errorf("autostart: %w", err))
		} else {
			req.Events.logf(LevelInfo, "autostart entry removed")
		}
	}
	if req.IndicatorRule != nil {
		if *req.IndicatorRule {
			if err := desktop.InstallRule(ctx); err != nil {
				errs = append(errs, fmt.Errorf("indicator rule: %w", err))
			} else {
				req.Events.logf(LevelInfo, "indicator window rule installed")
			}
		} else if _, err := desktop.RemoveRule(ctx); err != nil {
			errs = append(errs, fmt.Errorf("indicator rule: %w", err))
		} else {
			req.Events.logf(LevelInfo, "indicator window rule removed")
		}
	}
	if req.VolumeKeys != nil {
		if *req.VolumeKeys {
			if err := desktop.InstallVolumeKeys(ctx); err != nil {
				errs = append(errs, fmt.Errorf("volume keys: %w", err))
			} else {
				req.Events.logf(LevelInfo, "the volume keys run ototo")
			}
		} else if _, err := desktop.RemoveVolumeKeys(ctx); err != nil {
			errs = append(errs, fmt.Errorf("volume keys: %w", err))
		} else {
			req.Events.logf(LevelInfo, "the volume keys are given back")
		}
	}
	return DesktopStatus(ctx), errors.Join(errs...)
}
