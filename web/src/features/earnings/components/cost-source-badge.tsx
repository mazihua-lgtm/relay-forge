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
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'

import { EARNINGS_COST_SOURCE_CONFIG } from '../constants'
import type { EarningsCostSource } from '../types'

interface CostSourceBadgeProps {
  source: EarningsCostSource
}

/** Says whether a cost figure was recorded, estimated, or a mix of both. */
export function CostSourceBadge(props: CostSourceBadgeProps) {
  const { t } = useTranslation()
  const config = EARNINGS_COST_SOURCE_CONFIG[props.source]

  return (
    <Tooltip>
      <TooltipTrigger render={<span className='inline-flex' />}>
        <StatusBadge
          label={t(config.labelKey)}
          variant={config.variant}
          copyable={false}
        />
      </TooltipTrigger>
      <TooltipContent>{t(config.descriptionKey)}</TooltipContent>
    </Tooltip>
  )
}
