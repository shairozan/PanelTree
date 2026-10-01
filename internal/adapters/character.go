package adapters

import (
	"github.com/shairozan/PanelTree/model"
	"strings"
)

// Character metadata supplements prompts. Published artwork is supplied
// separately through the frozen image-input contract when explicitly selected.
func characterPrompt(c *model.ResolvedCharacter) string {
	parts := []string{c.Description}
	if len(c.Palette) > 0 {
		parts = append(parts, "palette: "+strings.Join(c.Palette, ", "))
	}
	for _, s := range []*model.CharacterState{c.Costume, c.Expression, c.Pose} {
		if s != nil {
			parts = append(parts, s.Description)
		}
	}
	for _, p := range c.Props {
		parts = append(parts, p.Description)
	}
	return "\nCharacter: " + strings.Join(parts, "; ")
}

func CharacterLimitations() []string {
	return []string{"reference-images: provenance only; image conditioning unsupported", "exact-props: description only; exact geometry unsupported", "pose-conditioning: description only; pose control unsupported", "identity-consistency: not guaranteed"}
}
