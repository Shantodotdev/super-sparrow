import { seedMilestones } from "../data/milestones";
import { seededQuery } from "./seeded";

export const milestonesQuery = seededQuery("milestones", seedMilestones);
