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
import { api } from '@/lib/api'

import type {
  ClientResponse,
  ClientsResponse,
  EndUserBlocksResponse,
} from './types'

export async function getClients(params?: {
  page?: number
  pageSize?: number
  keyword?: string
}): Promise<ClientsResponse> {
  const res = await api.get('/api/client/', {
    params: {
      p: params?.page ?? 1,
      page_size: params?.pageSize ?? 20,
      keyword: params?.keyword ?? '',
    },
  })
  return res.data
}

export async function createClient(payload: {
  name: string
  budget_quota: number
  remark: string
}): Promise<ClientResponse> {
  const res = await api.post('/api/client/', payload)
  return res.data
}

export async function updateClient(
  id: number,
  payload: {
    name?: string
    budget_quota?: number
    status?: number
    remark?: string
  }
): Promise<ClientResponse> {
  const res = await api.put(`/api/client/${id}`, payload)
  return res.data
}

export async function deleteClient(id: number): Promise<ClientResponse> {
  const res = await api.delete(`/api/client/${id}`)
  return res.data
}

export async function getMyEndUserBlocks(): Promise<EndUserBlocksResponse> {
  const res = await api.get('/api/end_user/blocks/self')
  return res.data
}

export async function clearMyEndUserBlock(params: {
  tokenId: number
  model: string
  endUser: string
}): Promise<{ success: boolean; message?: string }> {
  const res = await api.delete('/api/end_user/blocks/self', {
    params: {
      token_id: params.tokenId,
      model: params.model,
      end_user: params.endUser,
    },
  })
  return res.data
}
