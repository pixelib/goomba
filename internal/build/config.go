package build

type Config struct {
	WorkDir      string
	OutputBase   string
	Platforms    []string
	Archs        []string
	NoParallel   bool
	NoTui        bool
	NoValidation bool
	Strict       bool
	Verbose      bool
	JavaHome     string
	GoArgs       []string
	ValidateCmd  []string
	GoVersion    string
	CgoEnabled   bool
}
