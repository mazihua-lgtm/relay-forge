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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight, Plus, Search } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { formatQuotaWithCurrency } from '@/lib/currency'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import {
  clearMyEndUserBlock,
  createClient,
  deleteClient,
  getClients,
  getMyEndUserBlocks,
  updateClient,
} from './api'
import { ClientFormDialog } from './components/client-form-dialog'
import {
  CLIENT_STATUS_DISABLED,
  CLIENT_STATUS_ENABLED,
  type ClientUsage,
  type EndUserBlock,
} from './types'

const PAGE_SIZE = 12

export function Clients() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [createOpen, setCreateOpen] = useState(false)
  const [pendingDelete, setPendingDelete] = useState<ClientUsage | null>(null)
  const [page, setPage] = useState(1)
  const [keywordInput, setKeywordInput] = useState('')
  const [keyword, setKeyword] = useState('')

  const clientsQuery = useQuery({
    queryKey: ['clients', page, keyword],
    queryFn: async () => {
      const response = requireServerSuccess(
        await getClients({ page, pageSize: PAGE_SIZE, keyword })
      )
      return (
        response.data ?? { items: [], total: 0, page: 1, page_size: PAGE_SIZE }
      )
    },
  })

  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: ['clients'] })

  const blocksQuery = useQuery({
    queryKey: ['end-user-blocks'],
    queryFn: async () => {
      const response = requireServerSuccess(await getMyEndUserBlocks())
      return response.data ?? []
    },
  })

  const unblockMutation = useMutation({
    mutationFn: (block: EndUserBlock) =>
      clearMyEndUserBlock({
        tokenId: block.token_id,
        model: block.model,
        endUser: block.end_user,
      }),
    onSuccess: async (response) => {
      requireServerSuccess(response)
      toast.success(t('Unblocked'))
      await queryClient.invalidateQueries({ queryKey: ['end-user-blocks'] })
    },
    onError: (error) => handleServerError(error, t('Failed to unblock')),
  })

  const createMutation = useMutation({
    mutationFn: createClient,
    onSuccess: async (response) => {
      requireServerSuccess(response)
      toast.success(t('Client created'))
      setCreateOpen(false)
      await invalidate()
    },
    onError: (error) => handleServerError(error, t('Failed to create client')),
  })

  const statusMutation = useMutation({
    mutationFn: (client: ClientUsage) =>
      updateClient(client.id, {
        status:
          client.status === CLIENT_STATUS_ENABLED
            ? CLIENT_STATUS_DISABLED
            : CLIENT_STATUS_ENABLED,
      }),
    onSuccess: async (response) => {
      requireServerSuccess(response)
      await invalidate()
    },
    onError: (error) => handleServerError(error, t('Failed to update client')),
  })

  const deleteMutation = useMutation({
    mutationFn: (id: number) => deleteClient(id),
    onSuccess: async (response) => {
      requireServerSuccess(response)
      toast.success(t('Client deleted'))
      setPendingDelete(null)
      await invalidate()
    },
    onError: (error) => handleServerError(error, t('Failed to delete client')),
  })

  const clients = clientsQuery.data?.items ?? []
  const total = clientsQuery.data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  const submitSearch = () => {
    setPage(1)
    setKeyword(keywordInput.trim())
  }

  return (
    <div className='space-y-6'>
      <div className='flex flex-wrap items-start justify-between gap-4'>
        <div className='space-y-1'>
          <h1 className='text-2xl font-semibold tracking-tight'>
            {t('Clients')}
          </h1>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Downstream customers you resell to. Give each one a budget, issue keys under it, and the margin is the budget minus what they used.'
            )}
          </p>
        </div>
        <div className='flex flex-wrap items-center gap-2'>
          <form
            className='flex items-center gap-2'
            onSubmit={(event) => {
              event.preventDefault()
              submitSearch()
            }}
          >
            <Input
              value={keywordInput}
              onChange={(event) => setKeywordInput(event.target.value)}
              placeholder={t('Search by name')}
              className='w-48'
            />
            <Button type='submit' variant='outline' size='icon'>
              <Search className='size-4' />
              <span className='sr-only'>{t('Search')}</span>
            </Button>
          </form>
          <Button onClick={() => setCreateOpen(true)}>
            <Plus className='mr-2 size-4' />
            {t('New client')}
          </Button>
        </div>
      </div>

      {clientsQuery.isLoading && <LoadingState />}
      {clientsQuery.isError && (
        <ErrorState
          title={t('Could not load clients')}
          onRetry={() => clientsQuery.refetch()}
        />
      )}
      {!clientsQuery.isLoading &&
        !clientsQuery.isError &&
        clients.length === 0 && (
          <EmptyState
            title={t('No clients yet')}
            description={t(
              'Create a client, then issue API keys under it to track their usage and margin.'
            )}
          />
        )}

      <div className='grid gap-4 md:grid-cols-2 xl:grid-cols-3'>
        {clients.map((client) => (
          <ClientCard
            key={client.id}
            client={client}
            onToggleStatus={() => statusMutation.mutate(client)}
            onDelete={() => setPendingDelete(client)}
            busy={statusMutation.isPending}
          />
        ))}
      </div>

      {total > PAGE_SIZE && (
        <div className='flex items-center justify-end gap-3'>
          <span className='text-muted-foreground text-sm'>
            {t('Page {{page}} of {{total}}', { page, total: totalPages })}
          </span>
          <Button
            variant='outline'
            size='icon'
            disabled={page <= 1}
            onClick={() => setPage((current) => Math.max(1, current - 1))}
          >
            <ChevronLeft className='size-4' />
            <span className='sr-only'>{t('Previous page')}</span>
          </Button>
          <Button
            variant='outline'
            size='icon'
            disabled={page >= totalPages}
            onClick={() =>
              setPage((current) => Math.min(totalPages, current + 1))
            }
          >
            <ChevronRight className='size-4' />
            <span className='sr-only'>{t('Next page')}</span>
          </Button>
        </div>
      )}

      {(blocksQuery.data?.length ?? 0) > 0 && (
        <Card>
          <CardHeader className='space-y-1'>
            <CardTitle className='text-base'>
              {t('Blocked end users')}
            </CardTitle>
            <p className='text-muted-foreground text-sm'>
              {t(
                'End users suspended after repeated policy refusals upstream. Unblocking lets them try again immediately.'
              )}
            </p>
          </CardHeader>
          <CardContent className='space-y-2'>
            {blocksQuery.data?.map((block) => (
              <div
                key={`${block.token_id}:${block.model}:${block.end_user}`}
                className='flex flex-wrap items-center justify-between gap-2 rounded-lg border p-3'
              >
                <div className='space-y-0.5'>
                  <p className='text-sm font-medium'>{block.end_user}</p>
                  <p className='text-muted-foreground text-xs'>
                    {block.model} ·{' '}
                    {t('{{count}} refusals', { count: block.refusals })} ·{' '}
                    {t('{{minutes}} min left', {
                      minutes: Math.max(
                        0,
                        Math.ceil(
                          (block.blocked_until * 1000 - Date.now()) / 60000
                        )
                      ),
                    })}
                  </p>
                </div>
                <Button
                  size='sm'
                  variant='outline'
                  disabled={unblockMutation.isPending}
                  onClick={() => unblockMutation.mutate(block)}
                >
                  {t('Unblock')}
                </Button>
              </div>
            ))}
          </CardContent>
        </Card>
      )}

      <ClientFormDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        isSubmitting={createMutation.isPending}
        onSubmit={(values) =>
          createMutation.mutate({
            name: values.name,
            budget_quota: values.budget,
            remark: values.remark,
          })
        }
      />

      <ConfirmDialog
        open={Boolean(pendingDelete)}
        onOpenChange={(open) => {
          if (!open) setPendingDelete(null)
        }}
        title={t('Delete client')}
        desc={t(
          'This removes {{name}}. A client that still has API keys cannot be deleted.',
          { name: pendingDelete?.name ?? '' }
        )}
        destructive
        isLoading={deleteMutation.isPending}
        handleConfirm={() => {
          if (pendingDelete) deleteMutation.mutate(pendingDelete.id)
        }}
      />
    </div>
  )
}

interface ClientCardProps {
  client: ClientUsage
  busy: boolean
  onToggleStatus: () => void
  onDelete: () => void
}

function ClientCard(props: ClientCardProps) {
  const { t } = useTranslation()
  const enabled = props.client.status === CLIENT_STATUS_ENABLED
  const margin = props.client.budget_quota - props.client.used_quota
  const overBudget = margin < 0

  return (
    <Card>
      <CardHeader className='flex flex-row items-center justify-between gap-2 space-y-0'>
        <CardTitle className='truncate text-base'>
          {props.client.name}
        </CardTitle>
        <StatusBadge
          label={enabled ? t('Enabled') : t('Disabled')}
          variant={enabled ? 'success' : 'neutral'}
          copyable={false}
        />
      </CardHeader>
      <CardContent className='space-y-4'>
        <dl className='grid grid-cols-2 gap-3 text-sm'>
          <div>
            <dt className='text-muted-foreground text-xs'>{t('Budget')}</dt>
            <dd className='font-semibold'>
              {formatQuotaWithCurrency(props.client.budget_quota)}
            </dd>
          </div>
          <div>
            <dt className='text-muted-foreground text-xs'>{t('Used')}</dt>
            <dd className='font-semibold'>
              {formatQuotaWithCurrency(props.client.used_quota)}
            </dd>
          </div>
          <div>
            <dt className='text-muted-foreground text-xs'>{t('Margin')}</dt>
            <dd
              className={
                overBudget ? 'text-destructive font-semibold' : 'font-semibold'
              }
            >
              {formatQuotaWithCurrency(margin)}
            </dd>
          </div>
          <div>
            <dt className='text-muted-foreground text-xs'>{t('API keys')}</dt>
            <dd className='font-semibold'>{props.client.key_count}</dd>
          </div>
        </dl>

        {overBudget && (
          <p className='text-destructive text-xs font-medium'>
            {t('Over budget: this client has used more than it paid for.')}
          </p>
        )}

        {props.client.remark && (
          <p className='text-muted-foreground line-clamp-2 text-xs'>
            {props.client.remark}
          </p>
        )}

        <div className='flex gap-2'>
          <Button
            size='sm'
            variant='outline'
            disabled={props.busy}
            onClick={props.onToggleStatus}
          >
            {enabled ? t('Disable') : t('Enable')}
          </Button>
          <Button
            size='sm'
            variant='outline'
            className='text-destructive'
            onClick={props.onDelete}
          >
            {t('Delete')}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
