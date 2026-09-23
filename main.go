package main

import (
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
	sdkplugin "github.com/lvfeng-z/library-squirrel-sdk/plugin"
)

func main() {
	handler := &LocalImportWorkFetcher{}

	sdkplugin.Serve(
		sdkplugin.WithWorkFetcher(handler),
		sdkplugin.WithActivate(func(ctx sdkdto.PluginContext) {
			Activate(ctx, handler)
		}),
		sdkplugin.WithShutdown(func() {
			Shutdown(handler)
		}),
	)
}
