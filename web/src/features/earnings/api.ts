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
import { api } from '@/lib/api'

import type {
  EarningsApiResponse,
  EarningsChannelRow,
  EarningsList,
  EarningsListQuery,
  EarningsModelRow,
  EarningsOverview,
  EarningsQuery,
} from './types'

// ============================================================================
// Earnings APIs (administrators only)
// ============================================================================

/** Revenue, cost and profit of the whole site over the window. */
export async function getEarningsOverview(
  params: EarningsQuery
): Promise<EarningsApiResponse<EarningsOverview>> {
  const res = await api.get('/api/admin/earnings/overview', { params })
  return res.data
}

/** The same figures per channel, largest revenue first. */
export async function getEarningsByChannel(
  params: EarningsListQuery
): Promise<EarningsApiResponse<EarningsList<EarningsChannelRow>>> {
  const res = await api.get('/api/admin/earnings/by-channel', { params })
  return res.data
}

/** The same figures per model, largest revenue first. */
export async function getEarningsByModel(
  params: EarningsListQuery
): Promise<EarningsApiResponse<EarningsList<EarningsModelRow>>> {
  const res = await api.get('/api/admin/earnings/by-model', { params })
  return res.data
}
