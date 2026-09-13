/**
 * Generated from schema/directives.yaml by cmd/gen-directives. Do not edit.
 */

export interface DirectiveDescriptor {
  name: string
  valueType: string
  values: string[]
  pattern: string | null
  arity: number
  required: boolean
  systemWritten: boolean
  uiHelper: string | null
}

export const DIRECTIVEID = 'id'
export const DIRECTIVESTATE = 'state'
export const DIRECTIVESCHEDULE = 'schedule'
export const DIRECTIVEONCE = 'once'
export const DIRECTIVETARGET = 'target'
export const DIRECTIVESKIPCOUNT = 'skip_count'
export const DIRECTIVESKIPUNTIL = 'skip_until'
export const DIRECTIVELASTTRIGGERED = 'last_triggered'
export const DIRECTIVELASTCOMPLETED = 'last_completed'
export const DIRECTIVEREASON = 'reason'

export const DIRECTIVES: DirectiveDescriptor[] = [
  {
    name: 'id',
    valueType: 'string',
    values: [],
    pattern: '^[a-z0-9_-]{1,64}$',
    arity: 1,
    required: true,
    systemWritten: false,
    uiHelper: null,
  },
  {
    name: 'state',
    valueType: 'enum',
    values: ['active', 'triggered', 'paused', 'completed', 'failed'],
    pattern: null,
    arity: 1,
    required: true,
    systemWritten: false,
    uiHelper: 'enum_dropdown',
  },
  {
    name: 'schedule',
    valueType: 'cron',
    values: [],
    pattern: null,
    arity: 1,
    required: false,
    systemWritten: false,
    uiHelper: 'cron_popover',
  },
  {
    name: 'once',
    valueType: 'datetime',
    values: [],
    pattern: null,
    arity: 1,
    required: false,
    systemWritten: false,
    uiHelper: 'datetime_picker',
  },
  {
    name: 'target',
    valueType: 'string',
    values: [],
    pattern: null,
    arity: 1,
    required: false,
    systemWritten: false,
    uiHelper: 'enum_dropdown',
  },
  {
    name: 'skip_count',
    valueType: 'integer',
    values: [],
    pattern: null,
    arity: 1,
    required: false,
    systemWritten: false,
    uiHelper: null,
  },
  {
    name: 'skip_until',
    valueType: 'datetime',
    values: [],
    pattern: null,
    arity: 1,
    required: false,
    systemWritten: false,
    uiHelper: 'datetime_picker',
  },
  {
    name: 'last_triggered',
    valueType: 'datetime',
    values: [],
    pattern: null,
    arity: 1,
    required: false,
    systemWritten: true,
    uiHelper: null,
  },
  {
    name: 'last_completed',
    valueType: 'datetime',
    values: [],
    pattern: null,
    arity: 1,
    required: false,
    systemWritten: true,
    uiHelper: null,
  },
  {
    name: 'reason',
    valueType: 'string',
    values: [],
    pattern: null,
    arity: 1,
    required: false,
    systemWritten: false,
    uiHelper: null,
  },
]
