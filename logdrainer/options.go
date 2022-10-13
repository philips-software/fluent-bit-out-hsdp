package logdrainer

type OptionFunc func(s *logDrainerStorer) error

func WithApplicationName(name string) OptionFunc {
	return func(s *logDrainerStorer) error {
		s.applicationName = name
		return nil
	}
}

func WithServerName(name string) OptionFunc {
	return func(s *logDrainerStorer) error {
		s.serverName = name
		return nil
	}
}

func WithDebug(debug bool) OptionFunc {
	return func(s *logDrainerStorer) error {
		s.debug = debug
		return nil
	}
}
