import type {ReactNode} from 'react';
import clsx from 'clsx';
import Link from '@docusaurus/Link';
import useBaseUrl from '@docusaurus/useBaseUrl';
import Layout from '@theme/Layout';
import Heading from '@theme/Heading';
import FeatureHighlights from '@site/src/components/FeatureHighlights';
import Reveal from '@site/src/components/Reveal';

import styles from './index.module.css';

function HomepageHeader() {
  const dashboard = useBaseUrl('/scout-dashboard.png');
  return (
    <header className={clsx('hero hero--primary', styles.heroBanner)}>
      <div className="container">
        <div className={clsx(styles.heroText, 'text--center')}>
          <Heading as="h1" className="hero__title">
            Migration discovery,
            <br />
            before migration risk.
          </Heading>
          <p className="hero__subtitle">
            Zyvor Scout is an Apache-2.0, read-only assessment tool for
            infrastructure teams planning migrations from virtualized
            environments to KVM/KubeVirt and other open platforms. It
            discovers or imports VM inventory, explains blockers, maps
            dependencies, and groups workloads into migration waves — one
            Go binary, no source-side changes.
          </p>
          <div className={styles.buttons}>
            <Link
              className="button button--secondary button--lg"
              to="https://github.com/zyvorai/scout#quick-start">
              Get Started
            </Link>
            <Link
              className="button button--outline button--lg button--secondary"
              to="https://github.com/zyvorai/scout">
              View on GitHub
            </Link>
          </div>
        </div>
      </div>
      <div className={styles.heroMediaWrap}>
        <img
          className={styles.heroMedia}
          src={dashboard}
          alt="Zyvor Scout dashboard — readiness scoring, dependency graph, and migration waves"
        />
        <p className={styles.heroMediaCaption}>
          The local dashboard, embedded into the binary — a real deployment, not a mockup.
        </p>
      </div>
    </header>
  );
}

function ProblemStatement() {
  return (
    <section className={styles.problem}>
      <div className="container">
        <Reveal className="row">
          <div className="col col--8 col--offset-2 text--center">
            <Heading as="h2" className={styles.sectionHeading}>
              Why Scout
            </Heading>
            <p>
              Migration projects often begin with spreadsheets that hide the
              hard parts: RDM/shared disks, vTPM, Secure Boot, passthrough
              devices, snapshot chains, legacy guests, and application
              dependencies. Those gaps don't show up until a migration is
              already underway.
            </p>
            <p>
              Scout turns those facts into an explainable readiness score
              and an execution-oriented plan before you commit to a
              migration. It's deliberately read-only —{' '}
              <strong>Scout assesses and plans, it does not modify source
              workloads or execute migrations.</strong> Every deduction in
              its scoring has a rule ID, a severity, a human explanation,
              and a recommendation, so "unknown" is never silently treated
              as "compatible."
            </p>
          </div>
        </Reveal>
      </div>
    </section>
  );
}

function TrustBand() {
  return (
    <section className={styles.trust}>
      <div className="container">
        <Reveal className={styles.trustGrid}>
          <div>
            <Heading as="h3" className={styles.sectionHeading}>
              Read-only, and honest about scope
            </Heading>
            <p>
              Apache-2.0. Single Go binary, no JavaScript build chain — the
              dashboard is compiled in via <code>go:embed</code>. Security
              headers and localhost-by-default web binding out of the box.
              Real GitHub Actions CI on every push (gofmt, build, unit and
              API tests).
            </p>
            <Link to="/docs/ARCHITECTURE">Read the architecture →</Link>
          </div>
          <div className={styles.trustBadges}>
            <img
              src="https://github.com/zyvorai/scout/actions/workflows/ci.yml/badge.svg"
              alt="CI status"
            />
            <img
              src="https://img.shields.io/badge/license-Apache--2.0-blue.svg"
              alt="Apache 2.0 license"
            />
          </div>
        </Reveal>
      </div>
    </section>
  );
}

function EnterpriseCTA() {
  return (
    <section className={styles.enterprise}>
      <div className="container text--center">
        <Reveal>
          <Heading as="h2" className={styles.sectionHeading}>
            Need production support or SLAs?
          </Heading>
          <p className={styles.enterpriseCopy}>
            Scout's core is Apache-2.0 and free to run in personal, lab, and
            commercial production use at no charge. Zyvor Enterprise adds
            production support, SLAs, and additional products for teams
            that need them.
          </p>
          <Link
            className="button button--primary button--lg"
            to="mailto:sales@zyvor.dev">
            Contact sales@zyvor.dev
          </Link>
        </Reveal>
      </div>
    </section>
  );
}

export default function Home(): ReactNode {
  return (
    <Layout
      title="Zyvor Scout — migration discovery and readiness"
      description="Apache-2.0, read-only assessment tool for infrastructure teams planning migrations from virtualized environments to KVM/KubeVirt and other open platforms.">
      <HomepageHeader />
      <main>
        <ProblemStatement />
        <Reveal>
          <FeatureHighlights />
        </Reveal>
        <TrustBand />
        <EnterpriseCTA />
      </main>
    </Layout>
  );
}
