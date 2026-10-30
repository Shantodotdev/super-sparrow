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
    <div className="max-w-[850px] [&_h2]:mb-[8px] [&_ol]:list-decimal [&_ol]:pl-[18px] [&_ol]:leading-[1.55] [&_ol]:text-[#b4c1d3] [&_p]:leading-[1.55] [&_p]:text-[#b4c1d3] [&_section]:mt-[24px] [&_section]:grid [&_section]:grid-cols-[38px_1fr] [&_section]:gap-[10px] [&_ul]:list-disc [&_ul]:pl-[18px] [&_ul]:leading-[1.55] [&_ul]:text-[#b4c1d3]">
      <section>
        <Icon name="target" className="plan-icon issue-icon-goal text-[24px]" />
        <div>
          <h2>Goal</h2>
          <p>{goal}</p>
        </div>
      </section>
      <section>
        <Icon name="list" className="plan-icon issue-icon-approach text-[24px]" />
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
        <Icon name="triangle-alert" className="plan-icon issue-icon-warning text-[24px]" />
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
        <Icon name="square-check" className="plan-icon issue-icon-checklist text-[24px]" />
        <div>
          <h2>Checklist</h2>
          {checklist.map((item, index) => (
            <label
              className="mx-0 my-[6px] flex items-center gap-[9px] font-normal text-[#b4c1d3] [&_input]:h-[15px] [&_input]:w-[15px] [&_input]:accent-[#4c75ff]"
              key={item.text}
            >
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
