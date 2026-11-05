import { createBrowserRouter, Link, Navigate, RouterProvider, useRouteError } from 'react-router'
import { SessionBoundary } from './providers'
import { DatabaseLayout, RequireAdmin, RequireSession, Shell } from './components/layout'
import { ErrorState, PageHeading } from './components/ui'
import Login from './pages/Login'
import Databases from './pages/Databases'
import { Explorer, TableViewer } from './pages/Explorer'
import Query from './pages/Query'
import { MigrationRoute, Migrations } from './pages/Migrations'
import Users from './pages/Users'
import Groups from './pages/Groups'
import Connections from './pages/Connections'

function NotFound() {
  return <><PageHeading title="Page not found" /><p className="helper-text">This page is not available.</p><Link className="button" to="/databases">Back to databases</Link></>
}

function RouteError() {
  const error = useRouteError()
  return <main className="content"><PageHeading title="Unable to display this page" /><ErrorState error={error} /><Link to="/databases" className="button">Back to databases</Link></main>
}

const router = createBrowserRouter([{
  element: <SessionBoundary />,
  errorElement: <RouteError />,
  children: [
    { path: '/login', element: <Login /> },
    { element: <RequireSession />, children: [{ element: <Shell />, children: [
      { index: true, element: <Navigate to="/databases" replace /> },
      { path: '/databases', element: <Databases /> },
      { path: '/databases/:dbId', element: <DatabaseLayout />, children: [
        { index: true, element: <Explorer /> },
        { path: 'schemas/:schema/tables/:table', element: <TableViewer /> },
        { path: 'query', element: <Query /> },
        { path: 'migrations', element: <Migrations /> },
        { path: 'migrations/new', element: <MigrationRoute /> },
        { path: 'migrations/:migrationId', element: <MigrationRoute /> },
      ] },
      { element: <RequireAdmin />, children: [
        { path: '/admin/users', element: <Users /> },
        { path: '/admin/groups', element: <Groups /> },
        { path: '/admin/databases', element: <Connections /> },
      ] },
      { path: '*', element: <NotFound /> },
    ] }] },
  ],
}])

export default function App() { return <RouterProvider router={router} /> }
