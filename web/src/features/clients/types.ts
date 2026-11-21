/*
Copyright (C) 2023-2026 QuantumNous

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

For commercial licensing, please contact support@quantumnous.com
*/
export const CLIENT_STATUS_ENABLED = 1
export const CLIENT_STATUS_DISABLED = 2

export interface Client {
  id: number
  user_id: number
  name: string
  budget_quota: number
  // Consumption of keys that were deleted; kept so the total never shrinks.
  used_quota_snapshot: number
  status: number
  remark: string
  created_time: number
  updated_time: number
}

export interface ClientUsage extends Client {
  key_count: number
  used_quota: number
}

export interface ClientsPage {
  items: ClientUsage[]
  total: number
  page: number
  page_size: number
}

export interface ClientsResponse {
  success: boolean
  message?: string
  data?: ClientsPage
}

export interface ClientResponse {
  success: boolean
  message?: string
  data?: Client
}

export interface ClientFormValues {
  name: string
  /** Budget in the display currency, converted to quota before sending. */
  budget: number
  remark: string
}

export interface EndUserBlock {
  token_id: number
  model: string
  end_user: string
  refusals: number
  blocked_until: number
}

export interface EndUserBlocksResponse {
  success: boolean
  message?: string
  data?: EndUserBlock[]
}
