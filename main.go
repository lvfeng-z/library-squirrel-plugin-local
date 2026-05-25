package main

import (
	pluginsdk "github.com/lvfeng-z/library-squirrel-plugin-sdk"
)

func main() {
	handler := &LocalImportTaskHandler{}

	pluginsdk.Serve(handler,
		pluginsdk.WithActivate(func(ctx pluginsdk.PluginContext) {
			Activate(ctx, handler)
		}),
	)
}
