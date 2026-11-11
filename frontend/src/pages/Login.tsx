import { useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Navigate, useNavigate, useSearchParams } from 'react-router'
import { api, returnPath } from '../api'
import { resetSession, useSession } from '../session'
import type { User } from '../types'
import { ErrorState, Icon, PageHeading } from '../components/ui'
import { useFormAction } from '../form-hooks'

export default function Login() {
  const session = useSession()
  const client = useQueryClient()
  const navigate = useNavigate()
  const [search] = useSearchParams()
  const [showPassword, setShowPassword] = useState(false)
  const [capsLock, setCapsLock] = useState(false)
  const password = useRef<HTMLInputElement>(null)
  const action = useFormAction()
  const next = returnPath(search.get('next'))
  if (session.data) return <Navigate to={next} replace />
  return <main id="main" className="login-layout"><section className="login-form-wrap" aria-label="Sign in">
    <div className="login-brand"><Icon name="database" /><span>Database Platform</span></div>
    <PageHeading title="Sign in" />
    <form id="login-form" onSubmit={event => {
      event.preventDefault()
      const data = new FormData(event.currentTarget)
      void action.run(async () => {
        try {
          const user = await api<User>('/api/v1/auth/login', { method: 'POST', data: { username: data.get('username'), password: data.get('password') } })
          if (password.current) password.current.value = ''
          await resetSession(client, user)
          await navigate(next, { replace: true })
        } catch (error) { password.current?.focus(); throw error }
      })
    }} aria-busy={action.busy}>
      <fieldset className="form-fields" disabled={action.busy}>
        <label htmlFor="username">Username</label><input id="username" name="username" autoComplete="username" maxLength={64} required autoFocus />
        <label htmlFor="password">Password</label>
        <div className="password-control"><input ref={password} id="password" name="password" type={showPassword ? 'text' : 'password'} autoComplete="current-password" required onKeyUp={event => setCapsLock(event.getModifierState('CapsLock'))} /><button type="button" className="password-toggle" aria-label={`${showPassword ? 'Hide' : 'Show'} password`} aria-pressed={showPassword} onClick={() => setShowPassword(!showPassword)}>{showPassword ? 'Hide' : 'Show'}</button></div>
        {capsLock && <p className="helper-text">Caps Lock is on.</p>}
        <ErrorState error={action.error} />
        <button className="button primary full-width" type="submit">{action.busy ? 'Signing in…' : 'Sign in'}</button>
      </fieldset>
    </form>
    <p className="login-help">Need an account? Contact your platform administrator.</p>
  </section></main>
}
