import type { FilterRule, AdvancedFilterPayload } from '../components/FileFilterModal.vue'

export function formatFilterRuleLabel(rule: FilterRule, policies: any[], t: (key: string, args?: any) => string): string {
  const fieldName = (() => {
    switch (rule.field) {
      case 'format': return t('filter.fieldFormat')
      case 'watermark_mode': return t('filter.fieldWatermark')
      case 'hotlink_mode': return t('filter.fieldHotlink')
      case 'policy_id': return t('filter.fieldPolicy')
      case 'size': return t('filter.fieldSize')
      case 'dimensions': return t('filter.fieldDimensions')
      case 'is_image': return t('filter.fieldType')
      case 'created_at': return t('filter.fieldTime')
      case 'temp_links': return t('filter.fieldTempLinks')
      case 'calls': return t('filter.fieldCalls')
      case 'uv': return t('filter.fieldUV')
      case 'display_name': return t('filter.fieldName')
      case 'public_path': return t('filter.fieldPath')
      case 'sha256': return t('filter.fieldSha256')
      case 'uploader': return t('filter.fieldUploader')
      default: return rule.field
    }
  })()

  switch (rule.field) {
    case 'format': {
      const opPrefix = (rule.op === 'neq' || rule.op === 'not_in') ? t('filter.opIsNot') : t('filter.opIs')
      const valStr = Array.isArray(rule.value) ? rule.value.join(', ') : rule.value
      return `${fieldName} ${opPrefix} ${String(valStr || '').toUpperCase()}`
    }
    case 'watermark_mode': {
      const valLabel = rule.value === 'on' ? t('upload.watermarkOn') : rule.value === 'off' ? t('upload.watermarkOff') : t('upload.watermarkInherit')
      const opPrefix = rule.op === 'neq' ? t('filter.opIsNot') : ''
      return `${fieldName}: ${opPrefix} ${valLabel}`
    }
    case 'hotlink_mode': {
      const valLabel = rule.value === 'on' ? t('files.hotlinkOn') : rule.value === 'off' ? t('files.hotlinkOff') : t('files.hotlinkInherit')
      const opPrefix = rule.op === 'neq' ? t('filter.opIsNot') : ''
      return `${fieldName}: ${opPrefix} ${valLabel}`
    }
    case 'policy_id': {
      const p = policies?.find(x => String(x.value) === String(rule.value))
      const pName = p ? p.label.replace(/^.*:\s*/, '') : rule.value
      const opPrefix = (rule.op === 'neq' || rule.op === 'not_in') ? t('filter.opIsNot') : ''
      return `${fieldName}: ${opPrefix} ${pName}`
    }
    case 'size': {
      const unit = rule.unit || 'MB'
      if (rule.op === 'between') {
        return `${fieldName}: ${rule.value} - ${rule.value_to} ${unit}`
      }
      const opSymbol = rule.op === 'gt' ? '>' : rule.op === 'gte' ? '≥' : rule.op === 'lt' ? '<' : rule.op === 'lte' ? '≤' : '='
      return `${fieldName} ${opSymbol} ${rule.value} ${unit}`
    }
    case 'dimensions': {
      if (rule.sub_type === 'aspect_ratio' || ['landscape', 'portrait', 'square'].includes(rule.op)) {
        const aspect = rule.op === 'landscape' ? t('filter.opLandscape') : rule.op === 'portrait' ? t('filter.opPortrait') : t('filter.opSquare')
        return `${fieldName}: ${aspect}`
      }
      const dimType = rule.sub_type === 'height' ? '高' : '宽'
      if (rule.op === 'between') {
        return `${dimType} ${rule.value} - ${rule.value_to} px`
      }
      const opSymbol = rule.op === 'gt' ? '>' : rule.op === 'gte' ? '≥' : rule.op === 'lt' ? '<' : '≤'
      return `${dimType} ${opSymbol} ${rule.value} px`
    }
    case 'is_image':
      return rule.op === 'image' ? t('filter.presetImages') : t('filter.presetNonImages')
    case 'created_at': {
      switch (rule.op) {
        case 'today': return `${fieldName}: ${t('filter.timeToday')}`
        case 'yesterday': return `${fieldName}: ${t('filter.timeYesterday')}`
        case '7d': return `${fieldName}: ${t('filter.time7Days')}`
        case '30d': return `${fieldName}: ${t('filter.time30Days')}`
        case 'this_month': return `${fieldName}: ${t('filter.timeThisMonth')}`
        case 'between': {
          const d1 = rule.value ? new Date(rule.value).toLocaleDateString() : ''
          const d2 = rule.value_to ? new Date(rule.value_to).toLocaleDateString() : ''
          return `${fieldName}: ${d1} ~ ${d2}`
        }
        case 'after': return `${fieldName} ≥ ${new Date(rule.value).toLocaleDateString()}`
        case 'before': return `${fieldName} ≤ ${new Date(rule.value).toLocaleDateString()}`
      }
      return `${fieldName}: ${rule.op}`
    }
    case 'temp_links':
      return rule.op === 'has_active' ? t('filter.opHasActive') : rule.op === 'has_any' ? t('filter.opHasAny') : t('filter.opNoTemp')
    case 'calls': {
      if (rule.op === 'is_zero') return `${fieldName}: ${t('filter.opIsZero')}`
      if (rule.op === 'gt_zero') return `${fieldName}: ${t('filter.opGtZero')}`
      const opSymbol = rule.op === 'gt' ? '>' : rule.op === 'gte' ? '≥' : rule.op === 'lt' ? '<' : rule.op === 'lte' ? '≤' : '='
      return `${fieldName} ${opSymbol} ${rule.value}`
    }
    case 'uv': {
      if (rule.op === 'is_zero') return `${fieldName}: ${t('filter.opIsZero')}`
      if (rule.op === 'gt_zero') return `${fieldName}: ${t('filter.opGtZero')}`
      const opSymbol = rule.op === 'gt' ? '>' : rule.op === 'gte' ? '≥' : rule.op === 'lt' ? '<' : rule.op === 'lte' ? '≤' : '='
      return `${fieldName} ${opSymbol} ${rule.value}`
    }
    default: {
      const opDesc = rule.op === 'contains' ? t('filter.opContains') : rule.op === 'not_contains' ? t('filter.opNotContains') : rule.op
      return `${fieldName} ${opDesc} "${rule.value}"`
    }
  }
}

export function formatFullFilterExpression(payload: AdvancedFilterPayload, policies: any[], t: (key: string, args?: any) => string): string {
  if (!payload || !payload.rules || payload.rules.length === 0) return ''
  const parts: string[] = []

  payload.rules.forEach((rule, idx) => {
    if (idx > 0) {
      const conn = (payload.rules[idx - 1].connector || 'AND').toUpperCase()
      parts.push(conn)
    }
    const pre = rule.prefix_parens || ''
    const suf = rule.suffix_parens || ''
    const label = formatFilterRuleLabel(rule, policies, t)
    parts.push(`${pre}${label}${suf}`)
  })

  return parts.join(' ')
}


