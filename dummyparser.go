package followparser

// dummyParser is a parser that does nothing. It is used for testing purposes.
type dummyParser struct{}

func (p *dummyParser) Parse(_ []byte) error {
	// do nothing
	return nil
}

func (p *dummyParser) Finish(_ float64) {
	// do nothing
}
