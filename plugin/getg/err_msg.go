package getg

import "log"

// when used properly with export_g, this file will be removed at runtime.
// presence of this file at runtime meaning rewrite does not work

func init() {
	log.Printf("Getg is MISSING! If you see this message, it means getg is being used without github.com/xhd2015/go-inspect/plugin/export_g, please check your rewrite config.")
}
