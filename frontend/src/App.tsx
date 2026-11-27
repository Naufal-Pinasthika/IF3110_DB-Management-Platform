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
import { DatabaseMigrationWorkspace, MigrationWorkspacePreview } from './pages/MigrationWorkspace'

// Give unknown paths a simple way back to the database list
function NotFound() {
  return (
    <>
      <PageHeading title="Page not found" />
      <p className="helper-text">This page is not available.</p>
      <Link className="button" to="/databases">
        Back to databases
      </Link>
    </>
  )
}

// Keep route failures inside the app shell when possible
function RouteError() {
  const error = useRouteError()
  return (
    <main className="content">
      <PageHeading title="Unable to display this page" />
      <ErrorState error={error} />
      <Link to="/databases" className="button">
        Back to databases
      </Link>
    </main>
  )
}

// Keep the milestone preview outside routes that require a signed in user
const router = createBrowserRouter([
  {
    path: '/preview/milestone-2',
    element: <MigrationWorkspacePreview />,
    errorElement: <RouteError />,
  },
  {
    element: <SessionBoundary />,
    errorElement: <RouteError />,
    children: [
      // Keep login public while session checks run in the background
      {
        path: '/login',
        element: <Login />,
      },
      // Keep database pages behind the signed in guard
      {
        element: <RequireSession />,
        children: [
          {
            element: <Shell />,
            children: [
              { index: true, element: <Navigate to="/databases" replace /> },
              { path: '/databases', element: <Databases /> },
              {
                path: '/databases/:dbId',
                element: <DatabaseLayout />,
                children: [
                  { index: true, element: <Explorer /> },
                  { path: 'schemas/:schema/tables/:table', element: <TableViewer /> },
                  { path: 'query', element: <Query /> },
                  { path: 'migrations', element: <Migrations /> },
                  // Put the new route before the id route so it stays specific
                  { path: 'migrations/new', element: <MigrationRoute /> },
                  { path: 'migrations/workflow', element: <DatabaseMigrationWorkspace /> },
                  { path: 'migrations/:migrationId', element: <MigrationRoute /> },
                ],
              },
              {
                // Keep user management behind the platform admin guard
                element: <RequireAdmin />,
                children: [
                  { path: '/admin', element: <Navigate to="/admin/users" replace /> },
                  { path: '/admin/users', element: <Users /> },
                  { path: '/admin/groups', element: <Groups /> },
                  { path: '/admin/databases', element: <Connections /> },
                ],
              },
              { path: '*', element: <NotFound /> },
            ],
          },
        ],
      },
    ],
  },
])

// Give the router to React after all route guards are ready
export default function App() {
  return <RouterProvider router={router} />
}
