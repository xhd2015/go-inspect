package internal

import (
	"github.com/xhd2015/go-inspect/rewrite/model"
	"github.com/xhd2015/go-inspect/rewrite/session"
)

type SourceImportRegistryRetriever interface {
	session.SourceImportRegistry

	// advanced properties, are protected
	GetModules() map[string][]*model.Module
}
