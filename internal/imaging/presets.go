package imaging

import (
	"sort"

	"github.com/UtopiaXC/UtopiaImages/internal/model"
)

type PresetMatcher struct {
	presets []model.ImagePreset
}

func NewPresetMatcher(presets []model.ImagePreset) *PresetMatcher {
	// Sort presets by width ascending
	sorted := make([]model.ImagePreset, len(presets))
	copy(sorted, presets)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Width < sorted[j].Width
	})
	return &PresetMatcher{presets: sorted}
}

// MatchNearest finds the smallest preset that covers both reqW and reqH, or the largest if none covers
func (m *PresetMatcher) MatchNearest(reqW, reqH int) *model.ImagePreset {
	if len(m.presets) == 0 {
		return nil
	}

	for _, p := range m.presets {
		if (reqW <= 0 || p.Width >= reqW) && (reqH <= 0 || p.Height == 0 || p.Height >= reqH) {
			return &p
		}
	}

	// If requested dimensions exceed all presets, return largest
	return &m.presets[len(m.presets)-1]
}

// FindByName finds preset by name like "thumb", "small"
func (m *PresetMatcher) FindByName(name string) *model.ImagePreset {
	for _, p := range m.presets {
		if p.Name == name {
			return &p
		}
	}
	return nil
}
