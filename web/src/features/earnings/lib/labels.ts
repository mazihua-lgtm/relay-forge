/*
Copyright (C) 2026 relay-forge contributors

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/
import type { EarningsChannelRow } from '../types'

/**
 * The display name of a channel row. A channel that was deleted after it
 * served requests has no name left, and logs without a channel carry id 0.
 */
export function channelLabel(
  row: EarningsChannelRow,
  deletedLabel: string,
  noChannelLabel: string
): string {
  if (row.channel_name) return row.channel_name
  return row.channel_id > 0 ? deletedLabel : noChannelLabel
}
