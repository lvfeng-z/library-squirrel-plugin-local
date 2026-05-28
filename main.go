package main

import (
	sdkdto "github.com/lvfeng-z/library-squirrel-plugin-sdk/dto"
	sdkplugin "github.com/lvfeng-z/library-squirrel-plugin-sdk/plugin"
)

func main() {
	handler := &LocalImportTaskHandler{}

	sdkplugin.Serve(handler,
		sdkplugin.WithActivate(func(ctx sdkdto.PluginContext) {
			Activate(ctx, handler)
		}),
	)
}
