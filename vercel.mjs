// One Git repository, two Vercel projects with independent build contracts.
// Origo API is marked at project level with ORIGO_DEPLOYMENT=1. The project
// ID is an additional safeguard if that env flag is unavailable during build.
const origo =
  process.env.ORIGO_DEPLOYMENT === '1' ||
  process.env.VERCEL_PROJECT_ID === 'prj_Jji2WKNFx1wpqCEKi35IMXVr7Wno';

export const config = origo
  ? {
      framework: null,
      functions: { 'api/*.go': { maxDuration: 300 } },
      rewrites: [{ source: '/mcp', destination: '/api/mcp' }],
      git: { deploymentEnabled: false }
    }
  : {
      installCommand: 'bun install --ignore-scripts',
      git: { deploymentEnabled: false }
    };
