import { reactive } from 'vue'

// Shared avatar state: addresses known to have no picture (so lists don't retry them) and a
// version that changes after the user uploads or removes their own photo.
export const avatars = reactive({ misses: new Set<string>(), version: 0 })

export function avatarChanged() {
  avatars.misses.clear()
  avatars.version = Date.now()
}
