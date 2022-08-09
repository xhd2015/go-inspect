package load

type Mod string

const (
	ModDefault Mod = ""
	ModVendor  Mod = "vendor"
	ModModule  Mod = "mod"
)

type FlagBuilder struct {
	// vendor mod
	Mod Mod
}

func (c *FlagBuilder) Build() []string {
	flags := make([]string, 0)
	if c.Mod != "" {
		flags = append(flags, "-mod="+string(c.Mod))
	}
	return flags
}
