UPDATE labels SET color = CASE color
  WHEN '#a7c0ff' THEN '#6c9bff'
  WHEN '#c5a8ff' THEN '#a77bff'
  WHEN '#a1e4cd' THEN '#3fd9a0'
  WHEN '#f2cf87' THEN '#ffc933'
  WHEN '#f2ad67' THEN '#ff9442'
  WHEN '#f2a9ae' THEN '#ff6b6b'
  WHEN '#8fd5da' THEN '#2ed3e6'
  WHEN '#a9b6c8' THEN '#9aa8bd'
  ELSE color
END;
