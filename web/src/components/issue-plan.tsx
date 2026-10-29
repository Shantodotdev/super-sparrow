import { sampleIssueDetails } from "../data/issue-details";
import { Icon } from "./icon";

type IssuePlanProps = {
  checks: boolean[];
  onToggleCheck: (index: number) => void;
};

/** The sample plan shown for issues that have no plan of their own. */
export function IssuePlan({ checks, onToggleCheck }: IssuePlanProps) {
  const { goal, approach, edgeCases, checklist } = sampleIssueDetails.plan;

  return (
    <div className="plan">
      <section>
        <Icon name="target" className="plan-icon issue-icon-goal" />
        <div>
          <h2>Goal</h2>
          <p>{goal}</p>
        </div>
      </section>
      <section>
        <Icon name="list" className="plan-icon issue-icon-approach" />
        <div>
          <h2>Approach</h2>
          <ol>
            {approach.map((step) => (
              <li key={step}>{step}</li>
            ))}
          </ol>
        </div>
      </section>
      <section>
        <Icon name="triangle-alert" className="plan-icon issue-icon-warning" />
        <div>
          <h2>Edge cases</h2>
          <ul>
            {edgeCases.map((edgeCase) => (
              <li key={edgeCase}>{edgeCase}</li>
            ))}
          </ul>
        </div>
      </section>
      <section>
        <Icon name="square-check" className="plan-icon issue-icon-checklist" />
        <div>
          <h2>Checklist</h2>
          {checklist.map((item, index) => (
            <label className="checkline" key={item.text}>
              <input
                type="checkbox"
                checked={checks[index]}
                onChange={() => onToggleCheck(index)}
              />
              {item.text}
            </label>
          ))}
        </div>
      </section>
    </div>
  );
}
