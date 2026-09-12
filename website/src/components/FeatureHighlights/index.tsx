import type {ReactNode} from 'react';
import Link from '@docusaurus/Link';
import Heading from '@theme/Heading';
import styles from './styles.module.css';

type FeatureItem = {
  title: string;
  description: ReactNode;
  to: string;
};

const FeatureList: FeatureItem[] = [
  {
    title: 'Read-only discovery',
    description:
      'Demo inventory generation and a dependency-free VMware vCenter REST connector. Fields the source API doesn\'t expose are left unknown, never guessed.',
    to: '/docs/ARCHITECTURE',
  },
  {
    title: 'Explainable compatibility engine',
    description:
      'Ready / Review / Blocked classification, where every deduction carries a rule ID, severity, human explanation, and recommendation — not an opaque score.',
    to: '/docs/ARCHITECTURE',
  },
  {
    title: 'Dependency graph & migration waves',
    description:
      'Maps connected workloads and groups them into execution-oriented migration waves, so you plan around real application dependencies, not guesswork.',
    to: '/docs/ARCHITECTURE',
  },
  {
    title: 'Portable HTML report',
    description:
      'Generate a standalone assessment report you can hand to a migration team or a stakeholder without giving them access to the running tool.',
    to: '/docs/API',
  },
  {
    title: 'One binary, no JS build chain',
    description:
      'The Zyvor-branded dashboard is compiled into the executable via go:embed. Security headers and localhost-by-default binding out of the box.',
    to: '/docs/ARCHITECTURE',
  },
  {
    title: 'Deploy your way',
    description:
      'Docker/Podman (Compose + Quadlet), a remote systemd deploy + smoke script, a Helm chart, and Kustomize manifests — all first-class.',
    to: '/docs/DEPLOY',
  },
];

function Feature({title, description, to}: FeatureItem) {
  return (
    <div className="col col--4">
      <Link to={to} className={styles.card}>
        <Heading as="h3">{title}</Heading>
        <p>{description}</p>
      </Link>
    </div>
  );
}

export default function FeatureHighlights(): ReactNode {
  return (
    <section className={styles.features}>
      <div className="container">
        <div className="row">
          {FeatureList.map((props, idx) => (
            <Feature key={idx} {...props} />
          ))}
        </div>
      </div>
    </section>
  );
}
