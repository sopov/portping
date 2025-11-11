package probe

import "github.com/sopov/portping/internal/models"

type Preset struct {
	Proto         models.Proto
	Port          string
	UDPPayloadHex string
}

var Predefined = map[string]Preset{
	// Common UDP ports
	"dns":  {Proto: models.UDP, Port: "53", UDPPayloadHex: "0000010000000000000100000377777706676f6f676c6503636f6d0000010001"},
	"ntp":  {Proto: models.UDP, Port: "123", UDPPayloadHex: "1b0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"},
	"stun": {Proto: models.UDP, Port: "3478", UDPPayloadHex: "000100002112a442636363636363636363636363"},
	// Common TCP ports
	"ftp":      {Proto: models.TCP, Port: "21"},
	"ssh":      {Proto: models.TCP, Port: "22"},
	"smtp":     {Proto: models.TCP, Port: "25"},
	"http":     {Proto: models.TCP, Port: "80"},
	"pop3":     {Proto: models.TCP, Port: "110"},
	"imap":     {Proto: models.TCP, Port: "143"},
	"https":    {Proto: models.TCP, Port: "443"},
	"mysql":    {Proto: models.TCP, Port: "3306"},
	"postgres": {Proto: models.TCP, Port: "5432"},
}

func GetPreset(name string) (Preset, bool) {
	p, ok := Predefined[name]
	return p, ok
}
