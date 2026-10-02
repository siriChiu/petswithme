package main

import (
	"fmt"
	"strconv"
	"strings"
)

type SettingsForm struct {
	Size                                                                 int
	Activity                                                             ActivityLevel
	CPUEnabled                                                           bool
	EnterPercent, ExitPercent, EnterSeconds, ExitSeconds, StretchSeconds string
}

func ParseSettingsForm(base Settings, f SettingsForm) (Settings, error) {
	if f.Size != 96 && f.Size != 144 && f.Size != 192 {
		return base, fmt.Errorf("請選擇貓咪大小")
	}
	if !ValidActivity(f.Activity) {
		return base, fmt.Errorf("請選擇活動程度")
	}
	s := base
	s.Size = f.Size
	s.Activity = f.Activity
	s.Quiet = f.Activity == ActivityQuiet
	s.CPU.Enabled = f.CPUEnabled
	fields := []struct {
		name, value string
		dest        *float64
	}{{"開始門檻", f.EnterPercent, &s.CPU.EnterPercent}, {"結束門檻", f.ExitPercent, &s.CPU.ExitPercent}, {"開始持續秒數", f.EnterSeconds, &s.CPU.EnterSeconds}, {"結束持續秒數", f.ExitSeconds, &s.CPU.ExitSeconds}, {"伸懶腰間隔", f.StretchSeconds, &s.CPU.StretchEverySeconds}}
	for _, field := range fields {
		v, err := strconv.ParseFloat(strings.TrimSpace(field.value), 64)
		if err != nil || !finite(v) {
			return base, fmt.Errorf("%s請填有效數字", field.name)
		}
		*field.dest = v
	}
	if s.CPU.ExitPercent <= 0 || s.CPU.EnterPercent >= 100 || s.CPU.ExitPercent >= s.CPU.EnterPercent {
		return base, fmt.Errorf("門檻需介於0與100之間，結束門檻必須低於開始門檻")
	}
	if err := s.CPU.Validate(); err != nil {
		return base, fmt.Errorf("三個時間請填1到86400秒，門檻需符合上下限")
	}
	return s, nil
}
