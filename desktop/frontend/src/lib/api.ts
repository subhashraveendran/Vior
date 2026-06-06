// Central re-export of every Wails-bound App method. Screens import
// from here instead of the deep relative path '../../wailsjs/go/main/App'
// so a future rename or a Vite path alias (@/api) only touches one
// file. Also gives a single grep target for 'all native calls'.
export {
  StartServer, StopServer, GetServerStatus,
  GetConnectedClients, GetConfig, UpdateConfig,
  GetVersion, CheckPermissions, PickAndSendFile,
  GetMenuBarVisible, SetMenuBarVisible,
  HasAccessibility,
} from '../../wailsjs/go/main/App'

export { EventsOn, EventsEmit } from '../../wailsjs/runtime/runtime'
