package plugin

import (
	"os"
	"path/filepath"
	"strings"
)

// Do not pass editor credentials, loader injection variables, shell startup
// hooks or language package paths to extensions. Plugins get an explicit,
// reproducible environment and a disposable home instead.
func pluginEnvironment(scratch, executable string) []string {
	env := []string{
		"SUNBOOK_PLUGIN_API=1",
		"HOME=" + scratch,
		"TMPDIR=" + scratch + string(filepath.Separator),
		"TMP=" + scratch,
		"TEMP=" + scratch,
		"XDG_CACHE_HOME=" + scratch,
		"XDG_CONFIG_HOME=" + scratch,
		"XDG_DATA_HOME=" + scratch,
		"PATH=" + filepath.Dir(executable) + ":/usr/bin:/bin:/usr/sbin:/sbin:/opt/homebrew/bin:/usr/local/bin",
		"PYTHONNOUSERSITE=1",
		"PYTHONDONTWRITEBYTECODE=1",
		"TERM=dumb",
	}
	for _, key := range []string{"LANG", "LC_ALL", "LC_CTYPE", "TZ"} {
		if value, ok := os.LookupEnv(key); ok && !strings.ContainsRune(value, 0) {
			env = append(env, key+"="+value)
		}
	}
	return env
}
