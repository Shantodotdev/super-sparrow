package httpserver

import "strings"

// browserMarks and systemMarks are matched in order, because user agents name the engines they
// imitate: Edge also says Chrome, Chrome also says Safari, and iOS also says Mac OS X.
var (
	browserMarks = [][2]string{{"Edg/", "Edge"}, {"OPR/", "Opera"}, {"Firefox/", "Firefox"}, {"Chrome/", "Chrome"}, {"Safari/", "Safari"}}
	systemMarks  = [][2]string{{"Android", "Android"}, {"iPhone", "iOS"}, {"iPad", "iOS"}, {"Windows", "Windows"},
		{"Mac OS X", "macOS"}, {"CrOS", "ChromeOS"}, {"Linux", "Linux"}}
)

// deviceLabel names a device from its User-Agent header, such as "Firefox on Linux", for the
// list of signed-in devices.
func deviceLabel(userAgent string) string {
	browser, system := firstMark(userAgent, browserMarks), firstMark(userAgent, systemMarks)

	switch {
	case browser != "" && system != "":
		return browser + " on " + system
	case browser != "":
		return browser
	case system != "":
		return system
	}
	return "Unknown device"
}

func firstMark(userAgent string, marks [][2]string) string {
	for _, mark := range marks {
		if strings.Contains(userAgent, mark[0]) {
			return mark[1]
		}
	}
	return ""
}
