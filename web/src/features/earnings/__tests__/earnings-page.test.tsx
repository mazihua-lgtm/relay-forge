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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { Earnings } from '../index'
import type {
  EarningsChannelRow,
  EarningsCostSource,
  EarningsFigures,
  EarningsModelRow,
} from '../types'

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  resources: { en: { translation: {} } },
  initAsync: false,
})

// The default window is the last 30 days counted back from today, so the clock
// is pinned. Only Date is faked: the DOM and user-event keep real timers.
const NOW = new Date(2026, 10, 20, 15, 30)
const DEFAULT_WINDOW = { start_date: '2026-10-22', end_date: '2026-11-20' }
const clients: QueryClient[] = []

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date'] })
  vi.setSystemTime(NOW)
})

afterEach(() => {
  cleanup()
  clients.splice(0).forEach((client) => client.clear())
  vi.useRealTimers()
})

function figures(overrides: Partial<EarningsFigures> = {}): EarningsFigures {
  return {
    total_revenue: 2300,
    total_cost: 1450,
    gross_profit: 850,
    gross_margin: 0.3696,
    request_count: 1234,
    refund_quota: 0,
    cost_source: 'estimated',
    ...overrides,
  }
}

function channelRow(
  id: number,
  name: string,
  overrides: Partial<EarningsFigures> = {}
): EarningsChannelRow {
  return { ...figures(overrides), channel_id: id, channel_name: name }
}

function modelRow(
  name: string,
  overrides: Partial<EarningsFigures> = {}
): EarningsModelRow {
  return { ...figures(overrides), model_name: name }
}

interface MockOptions {
  costSource?: EarningsCostSource
  channels?: EarningsChannelRow[]
  channelTotal?: number
  models?: EarningsModelRow[]
}

/** The network boundary: the three earnings endpoints, answering as the server does. */
function mockEarningsApi(options: MockOptions = {}) {
  const costSource = options.costSource ?? 'estimated'
  const channels = options.channels ?? [
    channelRow(1, 'openai-main', { cost_source: costSource }),
    channelRow(2, 'claude-main', { cost_source: costSource }),
  ]
  const models = options.models ?? [
    modelRow('gpt-4o', { cost_source: costSource }),
  ]
  const envelope = (items: unknown[], total: number, page: number) => ({
    data: {
      success: true,
      data: {
        items,
        total,
        page,
        page_size: 10,
        cost_source: costSource,
        cost_ratio: 1.2,
        date_range: { start: '', end: '' },
      },
    },
  })

  return vi.spyOn(api, 'get').mockImplementation(async (url, config) => {
    const page = Number(paramsOf(config).p ?? 1)
    switch (url) {
      case '/api/admin/earnings/overview':
        return {
          data: {
            success: true,
            data: {
              ...figures({ cost_source: costSource, request_count: 4321 }),
              cost_ratio: 1.2,
              date_range: { start: '', end: '' },
            },
          },
        }
      case '/api/admin/earnings/by-channel':
        return envelope(channels, options.channelTotal ?? channels.length, page)
      case '/api/admin/earnings/by-model':
        return envelope(models, models.length, page)
      default:
        throw new Error(`unexpected GET ${url}`)
    }
  })
}

async function renderEarnings() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  const root = createRootRoute()
  const earnings = createRoute({
    getParentRoute: () => root,
    path: 'earnings/',
    component: Earnings,
  })
  // The notice links to the channels page.
  const channels = createRoute({
    getParentRoute: () => root,
    path: 'channels/',
    component: () => <div>channels page</div>,
  })
  const router = createRouter({
    routeTree: root.addChildren([earnings, channels]),
    history: createMemoryHistory({ initialEntries: ['/earnings/'] }),
  })
  await router.load()
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </I18nextProvider>
  )
}

/** The query parameters a request was sent with. */
function paramsOf(config: { params?: unknown } | undefined) {
  return (config?.params ?? {}) as Record<string, unknown>
}

function callsTo(get: ReturnType<typeof mockEarningsApi>, url: string) {
  return get.mock.calls.filter(([called]) => called === url)
}

const user = () => userEvent.setup({ advanceTimers: () => {} })

it('shows the totals and one row per channel for the last 30 days', async () => {
  const get = mockEarningsApi()
  await renderEarnings()

  expect(await screen.findByText('openai-main')).toBeInTheDocument()
  expect(screen.getByText('claude-main')).toBeInTheDocument()
  // The summary card and the table rows carry different request counts.
  expect(screen.getByText('4,321')).toBeInTheDocument()
  expect(screen.getAllByText('1,234')).toHaveLength(2)
  expect(callsTo(get, '/api/admin/earnings/overview')[0][1]?.params).toEqual(
    DEFAULT_WINDOW
  )
  expect(
    callsTo(get, '/api/admin/earnings/by-channel')[0][1]?.params
  ).toMatchObject({ ...DEFAULT_WINDOW, p: 1, page_size: 10 })
})

it('keeps the refunds and the gross margin on small screens', async () => {
  mockEarningsApi()
  await renderEarnings()

  await screen.findByText('openai-main')
  // The summary cards hide their details on small screens unless asked not to.
  for (const label of ['Refunds', 'Gross margin']) {
    // "Gross margin" is also a table column; only the card detail matters here.
    const inCards = screen
      .getAllByText(label)
      .filter((node) => !node.closest('table'))
    expect(inCards).toHaveLength(1)
    expect(inCards[0].closest('.hidden')).toBeNull()
  }
})

it('warns that the cost is estimated when no purchase ratio was recorded', async () => {
  mockEarningsApi({ costSource: 'estimated' })
  await renderEarnings()

  expect(await screen.findByText('The cost is estimated')).toBeInTheDocument()
  // The design system renders a link-styled button as <a role="button">.
  expect(
    screen.getByRole('button', { name: 'Set purchase ratios' })
  ).toHaveAttribute('href', '/channels')
})

it('says part of the cost is estimated when recorded and estimated costs mix', async () => {
  mockEarningsApi({ costSource: 'mixed' })
  await renderEarnings()

  expect(
    await screen.findByText('Part of the cost is estimated')
  ).toBeInTheDocument()
})

it('shows no estimate warning when every cost was recorded', async () => {
  mockEarningsApi({ costSource: 'recorded' })
  await renderEarnings()

  expect((await screen.findAllByText('Recorded')).length).toBeGreaterThan(0)
  expect(screen.queryByText('The cost is estimated')).not.toBeInTheDocument()
  expect(
    screen.queryByText('Part of the cost is estimated')
  ).not.toBeInTheDocument()
})

it('labels a deleted channel and a channel-less log instead of showing a blank name', async () => {
  mockEarningsApi({
    channels: [channelRow(7, ''), channelRow(0, '')],
  })
  await renderEarnings()

  expect(await screen.findByText('Deleted channel')).toBeInTheDocument()
  expect(screen.getByText('#7')).toBeInTheDocument()
  expect(screen.getByText('No channel')).toBeInTheDocument()
})

it('asks for the next page of channels when Next is chosen', async () => {
  const get = mockEarningsApi({
    channels: [channelRow(1, 'openai-main')],
    channelTotal: 25,
  })
  await renderEarnings()
  expect(await screen.findByText('Page 1 of 3')).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Previous page' })).toBeDisabled()

  await user().click(screen.getByRole('button', { name: 'Next page' }))

  expect(await screen.findByText('Page 2 of 3')).toBeInTheDocument()
  const pages = callsTo(get, '/api/admin/earnings/by-channel').map(
    ([, config]) => paramsOf(config).p
  )
  expect(pages).toEqual([1, 2])
})

it('keeps the report on its last page from running past the end', async () => {
  mockEarningsApi({
    channels: [channelRow(1, 'openai-main')],
    channelTotal: 11,
  })
  await renderEarnings()
  await screen.findByText('Page 1 of 2')

  await user().click(screen.getByRole('button', { name: 'Next page' }))

  await screen.findByText('Page 2 of 2')
  expect(screen.getByRole('button', { name: 'Next page' })).toBeDisabled()
})

it('switches to the per-model report with the same window', async () => {
  const get = mockEarningsApi({ models: [modelRow('gpt-4o')] })
  await renderEarnings()
  await screen.findByText('openai-main')

  await user().click(screen.getByRole('tab', { name: 'By model' }))

  expect(await screen.findByText('gpt-4o')).toBeInTheDocument()
  expect(screen.queryByText('openai-main')).not.toBeInTheDocument()
  expect(
    callsTo(get, '/api/admin/earnings/by-model')[0][1]?.params
  ).toMatchObject({ ...DEFAULT_WINDOW, p: 1 })
})

it('queries the whole window of a preset when one is chosen', async () => {
  const get = mockEarningsApi()
  await renderEarnings()
  await screen.findByText('openai-main')

  await user().click(screen.getByRole('button', { name: 'Last 7 days' }))

  await waitFor(() =>
    expect(
      callsTo(get, '/api/admin/earnings/overview').map(([, c]) => c?.params)
    ).toContainEqual({ start_date: '2026-11-14', end_date: '2026-11-20' })
  )
  await waitFor(() =>
    expect(
      callsTo(get, '/api/admin/earnings/by-channel').map(([, c]) => c?.params)
    ).toContainEqual(
      expect.objectContaining({
        start_date: '2026-11-14',
        end_date: '2026-11-20',
        p: 1,
      })
    )
  )
})

it('starts the report over on page 1 when the window changes', async () => {
  const get = mockEarningsApi({
    channels: [channelRow(1, 'openai-main')],
    channelTotal: 25,
  })
  await renderEarnings()
  await screen.findByText('Page 1 of 3')
  await user().click(screen.getByRole('button', { name: 'Next page' }))
  await screen.findByText('Page 2 of 3')

  await user().click(screen.getByRole('button', { name: 'Last 7 days' }))

  await waitFor(() =>
    expect(
      callsTo(get, '/api/admin/earnings/by-channel')
        .map(([, c]) => c?.params)
        .at(-1)
    ).toMatchObject({ start_date: '2026-11-14', p: 1 })
  )
  expect(await screen.findByText('Page 1 of 3')).toBeInTheDocument()
})

it('lets the operator retry a failed report and shows the rows once it succeeds', async () => {
  let failures = 1
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/admin/earnings/overview') {
      return {
        data: {
          success: true,
          data: { ...figures(), cost_ratio: 1.2, date_range: {} },
        },
      }
    }
    if (failures > 0) {
      failures -= 1
      return { data: { success: false, message: 'database is busy' } }
    }
    return {
      data: {
        success: true,
        data: {
          items: [channelRow(1, 'openai-main')],
          total: 1,
          page: 1,
          page_size: 10,
          cost_source: 'estimated',
          cost_ratio: 1.2,
          date_range: {},
        },
      },
    }
  })
  await renderEarnings()

  const retry = await screen.findByRole('button', { name: 'Retry' })
  await user().click(retry)

  expect(await screen.findByText('openai-main')).toBeInTheDocument()
})
